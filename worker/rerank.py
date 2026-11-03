"""One bounded local reranking operation at a time; no retrieval or credentials.

Only main() loads ML dependencies. Server/codecs are testable with stdlib scorers.
A disconnected caller never frees a still-running inference thread's reservation.
"""
import argparse
import hashlib
import importlib.metadata
import json
import logging
import math
import os
from pathlib import Path
import platform
import resource
import secrets
import select
import signal
import socket
import stat
import struct
import sys
import threading
import time

PROTOCOL = "concordance.local-rerank/v1"
INPUT_VERSION = "headings-notices-complete-body/v1"
MODEL = "BAAI/bge-reranker-v2-m3"
REVISION = "953dc6f6f85a1b2dbfca4c34a2796e7dde08d41e"
RUNTIME = "python3.14.2/torch2.15.0/transformers5.22.1/mps-f32-sdpa-b1-t2-1/v1"
MAX_REQUEST, MAX_GREETING, MAX_RESPONSE = 16 << 20, 16 << 10, 64 << 10
MAX_QUERY, MAX_TEXT, MAX_PAIRS = 4096, 128 << 10, 100
MAX_PAIR_TOKENS, MAX_TOTAL_TOKENS = 8192, 32768
READ_SECONDS, MAX_SECONDS, CONTROL_SECONDS = 5.0, 300.0, 0.25
ASSETS = {
    "config.json": (795, "13dcd6c31d9fec9d1d8e158702072f62d7fa7d312a64b9fe057bec9a08cfe41a"),
    "model.safetensors": (2271071852, "d9e3e081faff1eefb84019509b2f5558fd74c1a05a2c7db22f74174fcedb5286"),
    "special_tokens_map.json": (964, "8c785abebea9ae3257b61681b4e6fd8365ceafde980c21970d001e834cf10835"),
    "tokenizer.json": (17098273, "69564b696052886ed0ac63fa393e928384e0f8caada38c1f4864a9bfbf379c15"),
    "tokenizer_config.json": (1173, "7e4c1cc848840aeccdd763458c18dd525eb0f795c992e00ebe9c28554e7db2d4"),
}
LOCK_SHA256 = "46b2ddcb769f005da50bc9b8854060a656e3ef03503bc69324707c9bd15bea98"


class Rejected(Exception):
    def __init__(self, code="invalid"):
        self.code = code


class TimedOut(Rejected):
    def __init__(self):
        super().__init__("timeout")


class ModelFailure(Exception):
    pass


class Aborted(Rejected):
    def __init__(self):
        super().__init__("cancelled")


def _remaining(conn, deadline):
    remaining = deadline - time.monotonic()
    if remaining <= 0:
        raise TimedOut()
    conn.settimeout(remaining)


def _exact(conn, count, deadline):
    out = bytearray()
    while len(out) < count:
        _remaining(conn, deadline)
        chunk = conn.recv(count - len(out))
        if not chunk:
            raise Aborted()
        out.extend(chunk)
    return bytes(out)


def read_frame(conn, cap, deadline):
    n = struct.unpack("!I", _exact(conn, 4, deadline))[0]
    if n == 0 or n > cap:
        raise Rejected("limit")
    return _exact(conn, n, deadline)


def write_frame(conn, value, cap, deadline):
    body = json.dumps(value, ensure_ascii=False, allow_nan=False, separators=(",", ":")).encode("utf-8")
    if not 0 < len(body) <= cap:
        raise Rejected("limit")
    for part in (struct.pack("!I", len(body)), body):
        view = memoryview(part)
        while view:
            _remaining(conn, deadline)
            n = conn.send(view)
            if not n:
                raise Aborted()
            view = view[n:]


def _unique(items):
    out = {}
    for key, value in items:
        key.encode("utf-8", "strict")
        if key in out or value is None:
            raise Rejected()
        out[key] = value
    return out


def strict_json(body):
    """Bound depth/value work before constructing a JSON object graph."""
    text = body.decode("utf-8", "strict")
    depth, tokens = 0, 0
    quoted, escaped, scalar = False, False, False
    for char in text:
        if quoted:
            if escaped:
                escaped = False
            elif char == "\\":
                escaped = True
            elif char == '"':
                quoted = False
            continue
        if char == '"':
            quoted, scalar = True, False
            tokens += 1
        elif char in "[{":
            depth += 1
            tokens += 1
            scalar = False
        elif char in "]}":
            depth -= 1
            scalar = False
        elif char in ",:":
            scalar = False
        elif not char.isspace() and not scalar:
            scalar = True
            tokens += 1
        if depth > 8 or tokens > 2048:
            raise Rejected("limit")
    def constant(_):
        raise Rejected()
    value = json.loads(text, object_pairs_hook=_unique, parse_constant=constant)
    return value


def _fields(value, fields):
    if type(value) is not dict or set(value) != set(fields):
        raise Rejected()


def _text(value, cap):
    if type(value) is not str or "\x00" in value:
        raise Rejected()
    if len(value.encode("utf-8", "strict")) > cap:
        raise Rejected("limit")
    return value


def _hex(value, length):
    return type(value) is str and len(value) == length and all(c in "0123456789abcdef" for c in value)


def _field(value):
    body = value.encode("utf-8")
    return struct.pack("!I", len(body)) + body


def pair_id(index, query, text):
    return hashlib.sha256(b"concordance.rerank-pair/v1\x00" + struct.pack("!I", index) + _field(query) + _field(text)).hexdigest()


def fingerprint(query, pairs):
    digest = hashlib.sha256(b"concordance.rerank-input/v1\x00")
    for value in (INPUT_VERSION, REVISION, query):
        digest.update(_field(value))
    digest.update(struct.pack("!I", len(pairs)))
    for pair in pairs:
        digest.update(bytes.fromhex(pair["id"]))
        digest.update(_field(pair["text"]))
    return digest.hexdigest()


def validate_request(value, nonce):
    _fields(value, ("version", "nonce", "request_id", "input_sha256", "query", "timeout_ms", "pairs"))
    if value["version"] != PROTOCOL or value["nonce"] != nonce or not _hex(value["request_id"], 32) or not _hex(value["input_sha256"], 64):
        raise Rejected()
    query = _text(value["query"], MAX_QUERY)
    if not query or type(value["timeout_ms"]) is not int or not 0 < value["timeout_ms"] <= 300000:
        raise Rejected()
    pairs = value["pairs"]
    if type(pairs) is not list or not 1 <= len(pairs) <= MAX_PAIRS:
        raise Rejected("limit")
    for i, pair in enumerate(pairs):
        _fields(pair, ("id", "text"))
        text = _text(pair["text"], MAX_TEXT)
        if not _hex(pair["id"], 64) or pair["id"] != pair_id(i, query, text):
            raise Rejected()
    if value["input_sha256"] != fingerprint(query, pairs):
        raise Rejected()
    return value


def _tags():
    return {"model": MODEL, "revision": REVISION, "input_version": INPUT_VERSION, "runtime": RUNTIME}


class Server:
    """One acceptor plus at most one operation thread, with no application queue.

    The caller supplies a loaded scorer. A normal call retains only request-local
    state. After token/model resource errors the server drains and fails closed.
    """
    def __init__(self, path, scorer, read_seconds=READ_SECONDS):
        self.path = str(path)
        self.scorer = scorer
        self.read_seconds = read_seconds
        self.stopping = threading.Event()
        self.ready = threading.Event()
        self.active = None
        self.failed = False

    def stop(self):
        self.stopping.set()

    def _check(self, conn, deadline):
        if self.stopping.is_set():
            raise Rejected("stopping")
        if time.monotonic() >= deadline:
            raise TimedOut()
        readable, _, _ = select.select([conn], [], [], 0)
        if readable:
            data = conn.recv(1, socket.MSG_PEEK)
            if not data:
                raise Aborted()
            # One request per connection, not a pipelined second message.
            raise Rejected()

    def _handle(self, conn):
        nonce = secrets.token_hex(16)
        request = None
        reserved = time.monotonic()
        deadline = reserved + self.read_seconds
        try:
            write_frame(conn, dict(version=PROTOCOL, kind="ready", nonce=nonce, **_tags()), MAX_GREETING, deadline)
            request = validate_request(strict_json(read_frame(conn, MAX_REQUEST, deadline)), nonce)
            deadline = min(reserved + MAX_SECONDS, time.monotonic() + request["timeout_ms"] / 1000)
            lengths, total = [], 0
            # Complete token admission precedes the first model call.
            for pair in request["pairs"]:
                self._check(conn, deadline)
                try:
                    count = self.scorer.token_count(request["query"], pair["text"])
                except Exception:
                    raise ModelFailure() from None
                self._check(conn, deadline)
                if type(count) is not int or not 0 < count <= MAX_PAIR_TOKENS or count > MAX_TOTAL_TOKENS - total:
                    raise Rejected("limit")
                lengths.append(count)
                total += count
            scores = []
            for pair, count in zip(request["pairs"], lengths):
                self._check(conn, deadline)
                try:
                    value = self.scorer.score(request["query"], pair["text"], count)
                except Exception:
                    raise ModelFailure() from None
                # score must synchronize its actual compute before returning.
                self._check(conn, deadline)
                if type(value) not in (int, float) or not math.isfinite(value):
                    raise RuntimeError("nonfinite model score")
                scores.append({"id": pair["id"], "score": value, "tokens": count})
            self._check(conn, deadline)
            write_frame(conn, dict(version=PROTOCOL, kind="result", nonce=nonce, request_id=request["request_id"], input_sha256=request["input_sha256"], scores=scores, **_tags()), MAX_RESPONSE, deadline)
        except (Rejected, ValueError, UnicodeError, OSError, TimeoutError) as error:
            code = error.code if isinstance(error, Rejected) else "timeout" if isinstance(error, TimeoutError) else "invalid"
            self._error(conn, nonce, request, code)
        except Exception:
            # An uncertain model/device state cannot serve the next caller.
            self.failed = True
            self.stop()
            self._error(conn, nonce, request, "runtime")
        finally:
            conn.close()

    def _error(self, conn, nonce, request, code):
        value = {"version": PROTOCOL, "kind": "error", "nonce": nonce, "code": code}
        if type(request) is dict and _hex(request.get("request_id"), 32) and _hex(request.get("input_sha256"), 64):
            value.update(request_id=request["request_id"], input_sha256=request["input_sha256"])
        try:
            write_frame(conn, value, MAX_RESPONSE, time.monotonic() + CONTROL_SECONDS)
        except (OSError, Rejected):
            pass

    def serve(self):
        path = Path(self.path)
        parent = path.parent
        if not path.is_absolute() or len(os.fsencode(path)) > 103:
            raise ValueError("provide a short absolute Unix socket path")
        parent.mkdir(mode=0o700, parents=True, exist_ok=True)
        info = parent.lstat()
        if parent.resolve() != parent or not stat.S_ISDIR(info.st_mode) or stat.S_IMODE(info.st_mode) != 0o700 or info.st_uid != os.geteuid():
            raise ValueError("socket directory must be private and owned, without symlinks")
        if os.path.lexists(path):
            raise ValueError("socket path already exists; inspect the owner before recovery")
        listener = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        owned = None
        try:
            mask = os.umask(0o077)
            try:
                listener.bind(self.path)
            finally:
                os.umask(mask)
            os.chmod(path, 0o600)
            owned = path.lstat()
            listener.listen(1)
            listener.settimeout(0.1)
            self.ready.set()
            logging.info("Local reranking worker ready; private socket bound")
            while not self.stopping.is_set():
                try:
                    conn, _ = listener.accept()
                except socket.timeout:
                    continue
                if self.stopping.is_set() or self.active is not None and self.active.is_alive():
                    try:
                        write_frame(conn, {"version": PROTOCOL, "kind": "error", "code": "stopping" if self.stopping.is_set() else "busy"}, MAX_GREETING, time.monotonic() + CONTROL_SECONDS)
                    except (OSError, Rejected):
                        pass
                    finally:
                        conn.close()
                    continue
                if self.active is not None:
                    self.active.join()
                self.active = threading.Thread(target=self._handle, args=(conn,), daemon=False)
                try:
                    self.active.start()
                except Exception:
                    conn.close()
                    raise
        finally:
            listener.close()
            if self.active is not None:
                self.active.join()
            if owned is not None:
                try:
                    current = path.lstat()
                    if stat.S_ISSOCK(current.st_mode) and (current.st_dev, current.st_ino) == (owned.st_dev, owned.st_ino):
                        path.unlink()
                except FileNotFoundError:
                    pass


class ModelScorer:
    """The frozen local trial runtime. The model directory stays unchanged on load."""
    def __init__(self, model_dir):
        if platform.python_version() != "3.14.2":
            raise RuntimeError("worker requires the pinned Python runtime")
        lock = Path(__file__).with_name("requirements.txt")
        with lock.open("rb") as stream:
            if hashlib.file_digest(stream, "sha256").hexdigest() != LOCK_SHA256:
                raise RuntimeError("worker dependency lock differs from the frozen runtime")

        for line in lock.read_text().splitlines():
            name, separator, version = line.partition("==")
            if not separator or importlib.metadata.version(name) != version:
                raise RuntimeError("worker dependency version differs from the frozen runtime")
        path = Path(model_dir).resolve(strict=True)
        if {p.name for p in path.iterdir()} - set(ASSETS) - {"download-manifest.json"}:
            raise RuntimeError("unrecognized files in the pinned model directory")
        for name, (size, digest) in ASSETS.items():
            file = path / name
            info = file.lstat()
            if not stat.S_ISREG(info.st_mode) or info.st_size != size:
                raise RuntimeError("model asset kind or length mismatch")
            with file.open("rb") as stream:
                if hashlib.file_digest(stream, "sha256").hexdigest() != digest:
                    raise RuntimeError("model asset checksum mismatch")
        os.environ.update(HF_HUB_OFFLINE="1", TRANSFORMERS_OFFLINE="1", TOKENIZERS_PARALLELISM="false", PYTORCH_ENABLE_MPS_FALLBACK="0")
        import torch
        from transformers import AutoModelForSequenceClassification, AutoTokenizer
        if not torch.backends.mps.is_available():
            raise RuntimeError("this frozen worker requires the tested MPS device")
        self.torch = torch
        torch.set_num_threads(2)
        torch.set_num_interop_threads(1)
        torch.manual_seed(0)
        self.tokenizer = AutoTokenizer.from_pretrained(path, local_files_only=True, trust_remote_code=False, use_fast=True)
        self.model = AutoModelForSequenceClassification.from_pretrained(path, local_files_only=True, trust_remote_code=False, use_safetensors=True, dtype=torch.float32, attn_implementation="sdpa")
        self.model.eval()
        self.model.to("mps")
        self.score("readiness", "Body:\nConcordance worker readiness.", self.token_count("readiness", "Body:\nConcordance worker readiness."))

    def token_count(self, query, text):
        return len(self.tokenizer(query, text, truncation=False, add_special_tokens=True)["input_ids"])

    def score(self, query, text, count):
        torch = self.torch
        tokens = self.tokenizer(query, text, truncation=False, return_tensors="pt")
        if not 0 < count <= MAX_PAIR_TOKENS or tokens["input_ids"].shape[-1] != count:
            raise RuntimeError("tokenizer changed between admission and inference")
        tokens = {key: value.to("mps") for key, value in tokens.items()}
        try:
            with torch.inference_mode():
                logits = self.model(**tokens).logits
                if tuple(logits.shape) != (1, 1):
                    raise RuntimeError("unexpected model result shape")
                value = float(logits.float().item())
        finally:
            torch.mps.synchronize()
        del tokens, logits
        peak = resource.getrusage(resource.RUSAGE_SELF).ru_maxrss
        driver = torch.mps.driver_allocated_memory()
        if not math.isfinite(value) or peak > 6 << 30 or driver > 6 << 30:
            raise RuntimeError("model result or resource gate failed")
        return value


def main():
    parser = argparse.ArgumentParser(description="Serve the pinned local reranker on a private Unix socket")
    parser.add_argument("--socket", required=True)
    parser.add_argument("--model-dir", required=True)
    args = parser.parse_args()
    logging.basicConfig(level=logging.INFO, format="%(message)s")
    scorer = ModelScorer(args.model_dir)
    server = Server(args.socket, scorer)
    for sig in (signal.SIGINT, signal.SIGTERM):
        signal.signal(sig, lambda *_: server.stop())
    server.serve()
    if server.failed:
        raise RuntimeError("worker stopped after a runtime failure")


if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        raise SystemExit(130)
    except Exception:
        print("Local reranking worker failed; check runtime, model assets and private socket configuration", file=sys.stderr)
        raise SystemExit(1)
