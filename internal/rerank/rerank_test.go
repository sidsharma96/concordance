package rerank

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func testContext(t testing.TB) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	t.Cleanup(cancel)
	return ctx
}
func testBatch(t testing.TB) Batch {
	t.Helper()
	b, e := Prepare(testContext(t), "retry?", []Input{{Body: "Retry café safely.", Headings: []string{"Pods"}, Labels: []string{"Advice"}}, {Body: "Different passage."}})
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func shortDir(t testing.TB) string {
	t.Helper()
	root, e := filepath.Abs("../../.tools/tmp")
	if e != nil {
		t.Fatal(e)
	}
	if e = os.MkdirAll(root, 0o700); e != nil {
		t.Fatal(e)
	}
	dir, e := os.MkdirTemp(root, "rw-")
	if e != nil {
		t.Fatal(e)
	}
	dir, e = filepath.EvalSymlinks(dir)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}
func TestPrepareOwnsExactModelInputAndBindings(t *testing.T) {
	in := []Input{{Body: "Full\nbody", Headings: []string{"Pods"}, Labels: []string{"Note:"}}}
	b, e := Prepare(testContext(t), "why?", in)
	if e != nil || b.pairs[0].Text != "Heading: Pods\nNotice: Note:\nBody:\nFull\nbody" {
		t.Fatal(b, e)
	}
	in[0].Headings[0] = "changed"
	if strings.Contains(b.pairs[0].Text, "changed") {
		t.Fatal("retained mutable caller alias")
	}
	q, e := Prepare(testContext(t), "why else?", []Input{{Body: "Full\nbody", Headings: []string{"Pods"}, Labels: []string{"Note:"}}})
	if e != nil || q.fingerprint == b.fingerprint || q.pairs[0].ID == b.pairs[0].ID {
		t.Fatal("unbound query")
	}
	for _, input := range []Input{{Body: "\xff"}, {Body: "\x00"}, {Body: strings.Repeat("a", MaxTextBytes)}} {
		if _, e := Prepare(testContext(t), "q", []Input{input}); e == nil {
			t.Fatal("bad text accepted")
		}
	}
	if _, e := Prepare(testContext(t), "q", make([]Input, MaxPairs+1)); !errors.Is(e, ErrLimit) {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(testContext(t))
	cancel()
	if _, e := Prepare(ctx, "q", nil); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	empty, e := Prepare(testContext(t), "q", nil)
	if e != nil {
		t.Fatal(e)
	}
	if got, e := Score(testContext(t), "nonexistent", empty); e != nil || len(got.Scores) != 0 {
		t.Fatal("empty batch contacted worker", got, e)
	}
}
func TestFrameAdmissionPartialIOAndStrictJSON(t *testing.T) {
	for _, n := range []uint32{0, 1025} {
		var h [4]byte
		binary.BigEndian.PutUint32(h[:], n)
		if _, e := readFrame(bytes.NewReader(h[:]), 1024); !errors.Is(e, ErrLimit) {
			t.Fatal(e)
		}
	}
	for _, b := range [][]byte{{0, 0}, {0, 0, 0, 5, '{', '}'}} {
		if _, e := readFrame(bytes.NewReader(b), 100); !errors.Is(e, io.ErrUnexpectedEOF) {
			t.Fatal("truncated frame", e)
		}
	}
	var wire bytes.Buffer
	if e := writeFrame(&wire, []byte("café\n")); e != nil {
		t.Fatal(e)
	}
	body, e := readFrame(bufio.NewReaderSize(&wire, 1), 100)
	if e != nil || string(body) != "café\n" {
		t.Fatal(body, e)
	}
	for _, b := range []string{`{"version":"x","version":"y"}`, `{"version":null}`, `{"version":"\ud800"}`, `{"extra":1}`, `{"version":NaN}`, strings.Repeat("[", 9) + "0" + strings.Repeat("]", 9), "{} {}"} {
		var g greeting
		if e := decode(testContext(t), []byte(b), &g); e == nil {
			t.Fatal("invalid JSON/schema", b)
		}
	}
	var g greeting
	if e := decode(testContext(t), []byte("{\"version\":\"\xff\"}"), &g); e == nil {
		t.Fatal("invalid UTF8")
	}
}
func startStub(t *testing.T, hold bool) (string, io.WriteCloser, *bufio.Reader) {
	t.Helper()
	python, e := exec.LookPath("python3")
	if e != nil {
		t.Skip("Python stdlib is required for cross-language worker checks")
	}
	socket := filepath.Join(shortDir(t), "s")
	script, e := filepath.Abs("../../worker/test_rerank.py")
	if e != nil {
		t.Fatal(e)
	}
	args := []string{"-I", "-B", script, "--interop", socket}
	if hold {
		args = append(args, "--hold")
	}
	workerCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	cmd := exec.CommandContext(workerCtx, python, args...)
	in, e := cmd.StdinPipe()
	if e != nil {
		t.Fatal(e)
	}
	stdout, e := cmd.StdoutPipe()
	if e != nil {
		t.Fatal(e)
	}
	var diagnostics bytes.Buffer
	cmd.Stderr = &diagnostics
	if e = cmd.Start(); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		io.WriteString(in, "stop\n")
		in.Close()
		defer cancel()
		if e := cmd.Wait(); e != nil {
			t.Errorf("fixture worker: %v %s", e, diagnostics.String())
		}
	})
	events := bufio.NewReader(stdout)
	line, e := events.ReadString('\n')
	if e != nil || line != "LISTENING\n" {
		t.Fatalf("fixture ready %q %v %s", line, e, diagnostics.String())
	}
	return socket, in, events
}
func TestPythonInteropAndCancelledComputeReservation(t *testing.T) {
	socket, in, events := startStub(t, true)
	b := testBatch(t)
	ctx, cancel := context.WithCancel(testContext(t))
	done := make(chan error, 1)
	go func() { _, e := Score(ctx, socket, b); done <- e }()
	line, e := events.ReadString('\n')
	if e != nil || line != "COMPUTING\n" {
		t.Fatal(line, e)
	}
	cancel()
	select {
	case e := <-done:
		if !errors.Is(e, context.Canceled) {
			t.Fatal(e)
		}
	case <-testContext(t).Done():
		t.Fatal("client cancellation did not return")
	}
	if got, e := Score(testContext(t), socket, b); !errors.Is(e, ErrBusy) || !reflect.DeepEqual(got, Result{}) {
		t.Fatal("released active compute reservation", got, e)
	}
	io.WriteString(in, "release\n")
	line, e = events.ReadString('\n')
	if e != nil || line != "DRAINED\n" {
		t.Fatal(line, e)
	}
	got, e := Score(testContext(t), socket, b)
	if e != nil || len(got.Scores) != b.Len() {
		t.Fatal(got, e)
	}
	for i, p := range b.pairs {
		if got.Scores[i] != float64(utf8.RuneCountInString(p.Text)) || got.Tokens[i] != 3 {
			t.Fatal("interop changed text/binding", got)
		}
	}
	t.Log("Go/Python framed JSON preserves complete Unicode/context input; cancelled client leaves scorer busy until held compute drains; subsequent complete request succeeds")
}

// A trusted fake peer deliberately corrupts returned records; the client must
// return no usable scores even when some records are otherwise valid.
func TestClientRejectsMisboundOrIncompleteResults(t *testing.T) {
	for _, change := range []string{"order", "nonce", "request", "input", "model", "duplicate", "missing", "nilscore", "tokens", "total", "nonfinite", "unknown"} {
		t.Run(change, func(t *testing.T) {
			socket := filepath.Join(shortDir(t), "s")
			listener, e := net.Listen("unix", socket)
			if e != nil {
				t.Fatal(e)
			}
			defer listener.Close()
			if e = os.Chmod(socket, 0o600); e != nil {
				t.Fatal(e)
			}
			b := testBatch(t)
			if change == "total" {
				b, e = Prepare(testContext(t), "q", []Input{{Body: "one"}, {Body: "two"}, {Body: "three"}, {Body: "four"}, {Body: "five"}})
				if e != nil {
					t.Fatal(e)
				}
			}
			serverDone := make(chan error, 1)
			go func() {
				conn, e := listener.Accept()
				if e != nil {
					serverDone <- e
					return
				}
				defer conn.Close()
				conn.SetDeadline(time.Now().Add(5 * time.Second))
				hello := greeting{Version: Protocol, Kind: "ready", Nonce: strings.Repeat("a", 32), Model: Model, Revision: Revision, InputVersion: InputVersion, Runtime: Runtime}
				wire, _ := json.Marshal(hello)
				if e = writeFrame(conn, wire); e != nil {
					serverDone <- e
					return
				}
				wire, e = readFrame(conn, maxRequest)
				if e != nil {
					serverDone <- e
					return
				}
				var req request
				if e = json.Unmarshal(wire, &req); e != nil {
					serverDone <- e
					return
				}
				reply := response{Version: Protocol, Kind: "result", Nonce: hello.Nonce, RequestID: req.RequestID, InputSHA256: req.InputSHA256, Model: Model, Revision: Revision, InputVersion: InputVersion, Runtime: Runtime}
				for i, p := range b.pairs {
					v := float64(i)
					reply.Scores = append(reply.Scores, score{ID: p.ID, Score: &v, Tokens: 3})
				}
				switch change {
				case "order":
					slices.Reverse(reply.Scores)
				case "nonce":
					reply.Nonce = "b"
				case "request":
					reply.RequestID = "wrong"
				case "input":
					reply.InputSHA256 = "wrong"
				case "model":
					reply.Revision = "wrong"
				case "duplicate":
					reply.Scores[1] = reply.Scores[0]
				case "missing":
					reply.Scores = reply.Scores[:1]
				case "nilscore":
					reply.Scores[0].Score = nil
				case "tokens":
					reply.Scores[0].Tokens = MaxPairTokens + 1
				case "total":
					for i := range reply.Scores {
						reply.Scores[i].Tokens = MaxPairTokens
					}
				case "nonfinite":
					*reply.Scores[0].Score = math.Inf(1)
				}
				wire, e = json.Marshal(reply)
				if change == "nonfinite" {
					wire = []byte(`{"version":"` + Protocol + `","kind":"result","scores":[{"score":NaN}]}`)
					e = nil
				}
				if change == "unknown" {
					wire = append(wire[:len(wire)-1], []byte(`,"extra":1}`)...)
				}
				if e == nil {
					e = writeFrame(conn, wire)
				}
				serverDone <- e
			}()
			got, e := Score(testContext(t), socket, b)
			if change == "order" {
				if e != nil || !slices.Equal(got.Scores, []float64{0, 1}) {
					t.Fatal("did not restore candidate order", got, e)
				}
			} else if e == nil || !reflect.DeepEqual(got, Result{}) {
				t.Fatal("corrupt peer exposed scores", got, e)
			}
			if e := <-serverDone; e != nil {
				t.Fatal(e)
			}
		})
	}
}
