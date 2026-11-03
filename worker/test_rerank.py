"""Stdlib-only tests and an explicitly test-file-only interop fixture.

Production rerank.py has no fake-model option. No model/GPU/download is required.
"""
import importlib.util
import json
import os
from pathlib import Path
import socket
import struct
import sys
import tempfile
import threading
import time
import unittest

spec = importlib.util.spec_from_file_location("concordance_worker", Path(__file__).with_name("rerank.py"))
w = importlib.util.module_from_spec(spec)
spec.loader.exec_module(w)


class Fake:
    def __init__(self, hold=False, tokens=3, failure=None, increasing=False):
        self.entered = threading.Event()
        self.release = threading.Event()
        self.hold, self.tokens, self.failure = hold, tokens, failure
        self.calls = 0
        self.increasing = increasing

    def token_count(self, query, text):
        return self.tokens

    def score(self, query, text, count):
        self.calls += 1
        self.entered.set()
        if self.hold and not self.release.wait(10):
            raise RuntimeError("test latch was not released")
        if self.failure:
            raise self.failure
        return float(self.calls if self.increasing else len(text))


def request(hello, texts=("Body:\nRetry café safely.",), query="retry?"):
    pairs = [{"id": w.pair_id(i, query, text), "text": text} for i, text in enumerate(texts)]
    return dict(version=w.PROTOCOL, nonce=hello["nonce"], request_id="a" * 32,
                input_sha256=w.fingerprint(query, pairs), query=query, timeout_ms=5000, pairs=pairs)


def receive(conn, cap=w.MAX_RESPONSE):
    return json.loads(w.read_frame(conn, cap, time.monotonic() + 5))


def send(conn, value):
    w.write_frame(conn, value, w.MAX_REQUEST, time.monotonic() + 5)


class WorkerTests(unittest.TestCase):
    def start(self, scorer=None, read_seconds=5):
        # Canonical short paths also work with macOS's small sockaddr_un.
        root = Path(__file__).resolve().parent.parent / ".tools" / "tmp"
        root.mkdir(parents=True, exist_ok=True)
        tmp = tempfile.TemporaryDirectory(prefix="rw-", dir=root)
        self.addCleanup(tmp.cleanup)
        server = w.Server(Path(tmp.name) / "s", scorer or Fake(), read_seconds=read_seconds)
        thread = threading.Thread(target=server.serve)
        thread.start()
        def stop():
            server.stop()
            server.scorer.release.set()
            thread.join(10)
            self.assertFalse(thread.is_alive())
        self.addCleanup(stop)
        self.assertTrue(server.ready.wait(5))
        return server, thread

    def connect(self, server):
        conn = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        self.addCleanup(conn.close)
        conn.settimeout(5)
        conn.connect(server.path)
        return conn, receive(conn, w.MAX_GREETING)

    def test_roundtrip_binding_and_next_request(self):
        server, _ = self.start()
        for _ in range(2):
            conn, hello = self.connect(server)
            self.assertEqual(hello["kind"], "ready")
            value = request(hello)
            send(conn, value)
            reply = receive(conn)
            self.assertEqual(reply["kind"], "result")
            self.assertEqual(reply["input_sha256"], value["input_sha256"])
            self.assertEqual(reply["request_id"], value["request_id"])
            self.assertEqual(reply["scores"][0]["id"], value["pairs"][0]["id"])
            self.assertEqual(reply["scores"][0]["score"], len(value["pairs"][0]["text"]))
            conn.close()
            server.active.join(5)
        self.assertEqual(server.scorer.calls, 2)

    def test_cancel_keeps_reservation_until_compute_drains(self):
        fake = Fake(hold=True)
        server, _ = self.start(fake)
        conn, hello = self.connect(server)
        send(conn, request(hello))
        self.assertTrue(fake.entered.wait(5))
        conn.close()
        second, busy = self.connect(server)
        self.assertEqual(busy["code"], "busy")
        second.close()
        self.assertEqual(fake.calls, 1)
        fake.release.set()
        server.active.join(5)
        self.assertFalse(server.active.is_alive())
        third, ready = self.connect(server)
        self.assertEqual(ready["kind"], "ready")
        third.close()

    def test_shutdown_joins_held_compute_and_removes_owned_socket(self):
        fake = Fake(hold=True)
        server, thread = self.start(fake)
        conn, hello = self.connect(server)
        send(conn, request(hello))
        self.assertTrue(fake.entered.wait(5))
        server.stop()
        # The held inference itself is the ordering latch; no timed sleep.
        self.assertTrue(thread.is_alive())
        self.assertTrue(Path(server.path).exists())
        fake.release.set()
        thread.join(5)
        self.assertFalse(thread.is_alive())
        self.assertFalse(Path(server.path).exists())

    def test_stalled_header_expires_on_server_deadline(self):
        server, _ = self.start(read_seconds=0.05)
        conn, hello = self.connect(server)
        self.assertEqual(hello["kind"], "ready")
        # No request/header is sent. A bounded recv waits for the server's error.
        reply = receive(conn)
        self.assertEqual(reply["kind"], "error")
        self.assertEqual(reply["code"], "timeout")
        server.active.join(5)
        again, hello = self.connect(server)
        self.assertEqual(hello["kind"], "ready")
        again.close()
        self.assertEqual(server.scorer.calls, 0)

    def test_partial_header_and_body_use_same_server_deadline(self):
        for prefix in (b"\x00\x00", struct.pack("!I", 100) + b"{"):
            with self.subTest(prefix=prefix):
                server, _ = self.start(read_seconds=0.05)
                conn, _ = self.connect(server)
                conn.sendall(prefix)
                self.assertEqual(receive(conn)["code"], "timeout")
                server.active.join(5)
                self.assertEqual(server.scorer.calls, 0)

    def test_complete_token_admission_precedes_inference(self):
        server, _ = self.start(Fake(tokens=w.MAX_PAIR_TOKENS))
        conn, hello = self.connect(server)
        send(conn, request(hello, texts=tuple("Body:\n" + str(i) for i in range(5))))
        self.assertEqual(receive(conn)["code"], "limit")
        self.assertEqual(server.scorer.calls, 0)

    def test_model_valueerror_fails_closed(self):
        server, thread = self.start(Fake(failure=ValueError("test model failure")))
        conn, hello = self.connect(server)
        send(conn, request(hello))
        self.assertEqual(receive(conn)["code"], "runtime")
        thread.join(5)
        self.assertTrue(server.failed)
        self.assertFalse(thread.is_alive())

    def test_wrong_binding_is_rejected_before_model(self):
        server, _ = self.start()
        for field in ("nonce", "request_id", "input_sha256"):
            conn, hello = self.connect(server)
            value = request(hello)
            value[field] = "wrong"
            send(conn, value)
            self.assertEqual(receive(conn)["code"], "invalid")
            conn.close()
            server.active.join(5)
        self.assertEqual(server.scorer.calls, 0)

    def test_strict_json_and_schema(self):
        for body in (b'{"a":1,"a":2}', b'{"a":null}', b'{"a":NaN}', b'{"a":"\xff"}', b'[' * 9 + b'0' + b']' * 9):
            with self.subTest(body=body):
                with self.assertRaises((w.Rejected, ValueError, UnicodeError)):
                    w.strict_json(body)
        hello = {"nonce": "b" * 32}
        for change in (lambda v: v.update(timeout_ms=True), lambda v: v.update(extra=1), lambda v: v["pairs"][0].update(text="\ud800"), lambda v: v["pairs"][0].update(id="0" * 64)):
            value = request(hello)
            change(value)
            with self.assertRaises((w.Rejected, UnicodeError)):
                w.validate_request(value, hello["nonce"])

    def test_length_rejected_before_body_read_and_truncation(self):
        for length in (0, w.MAX_REQUEST + 1):
            a, b = socket.socketpair()
            with a, b:
                a.sendall(struct.pack("!I", length))
                with self.assertRaises(w.Rejected):
                    w.read_frame(b, w.MAX_REQUEST, time.monotonic() + 5)
        a, b = socket.socketpair()
        with a, b:
            a.sendall(struct.pack("!I", 4) + b'{}')
            a.shutdown(socket.SHUT_WR)
            with self.assertRaises(w.Aborted):
                w.read_frame(b, w.MAX_REQUEST, time.monotonic() + 5)

    def test_does_not_unlink_replaced_socket_path(self):
        server, thread = self.start()
        path = Path(server.path)
        path.unlink()
        path.write_text("replacement")
        server.stop()
        thread.join(5)
        self.assertEqual(path.read_text(), "replacement")


# Test-file-only entry point used by Go interoperability tests. No ML imports.
def interop(path, hold=False, increasing=False):
    fake = Fake(hold=hold, increasing=increasing)
    server = w.Server(path, fake)
    def controls():
        for line in sys.stdin:
            if line.strip() == "release":
                fake.release.set()
                if server.active is not None:
                    server.active.join(5)
                print("DRAINED", flush=True)
            elif line.strip() == "stop":
                server.stop()
                fake.release.set()
    threading.Thread(target=controls, daemon=True).start()
    def events():
        server.ready.wait()
        print("LISTENING", flush=True)
        if hold:
            fake.entered.wait()
            print("COMPUTING", flush=True)
    threading.Thread(target=events, daemon=True).start()
    try:
        server.serve()
    finally:
        server.stop()
        fake.release.set()


if __name__ == "__main__":
    if len(sys.argv) >= 3 and sys.argv[1] == "--interop":
        interop(sys.argv[2], hold="--hold" in sys.argv, increasing="--increasing" in sys.argv)
    else:
        unittest.main()
