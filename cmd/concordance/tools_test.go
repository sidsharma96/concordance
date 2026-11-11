package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Only the test binary understands these helper modes; production has no fake
// command, executable override or environment supplied by tool arguments.
func TestToolChild(t *testing.T) {
	mode := os.Getenv("CONCORDANCE_TOOL_TEST_CHILD")
	if mode == "" {
		return
	}
	switch mode {
	case "server":
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
		ready := os.NewFile(3, "ready")
		observed := make(chan *exec.Cmd, 1)
		_ = serveTools(ctx, os.Stdin, os.Stdout, toolsOptions{target: "t", limit: 1}, childCommand("wait", []*os.File{ready}, observed))
		select {
		case child := <-observed:
			if child.ProcessState == nil {
				os.Exit(9)
			}
		default:
			os.Exit(8)
		}

	case "wait":
		ready := os.NewFile(3, "ready")
		_, _ = ready.Write([]byte{1})
		_ = ready.Close()
		<-time.After(time.Hour)
	case "overflow":
		_, _ = os.Stdout.Write(bytes.Repeat([]byte("x"), 32768))
	case "failure":
		_, _ = os.Stdout.Write([]byte(`{"partial":true}`))
		_, _ = os.Stderr.Write([]byte("secret-provider-diagnostic"))
		os.Exit(7)
	default:
		i := 0
		for i < len(os.Args) && os.Args[i] != "--" {
			i++
		}
		args := os.Args[i+1:]
		format := "concordance.docs-search/v1"
		if args[0] == "read" {
			format = "concordance.docs-read/v1"
		}
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"format": format, "status": "ok", "arguments": args})
	}
	os.Exit(0)
}
func childCommand(mode string, extra []*os.File, observe chan<- *exec.Cmd) toolCommand {
	return func(ctx context.Context, args []string) *exec.Cmd {
		cmd := exec.CommandContext(ctx, os.Args[0], append([]string{"-test.run=^TestToolChild$", "--"}, args...)...)
		cmd.Env = append(os.Environ(), "CONCORDANCE_TOOL_TEST_CHILD="+mode)
		cmd.ExtraFiles = extra
		if observe != nil {
			observe <- cmd
		}
		return cmd
	}
}
func toolRequest(raw string) *mcp.CallToolRequest {
	return &mcp.CallToolRequest{Params: &mcp.CallToolParamsRaw{Arguments: json.RawMessage(raw)}}
}
func resultText(r *mcp.CallToolResult) string { return r.Content[0].(*mcp.TextContent).Text }
func newTestTools(command toolCommand) *agentTools {
	return &agentTools{base: context.Background(), options: toolsOptions{target: "kubernetes", limit: 5}, command: command}
}

func TestToolsArgumentsAndLiteralExecution(t *testing.T) {
	a := newTestTools(childCommand("success", nil, nil))
	query := "--help; $(echo surprise) `echo other`"
	raw, _ := json.Marshal(map[string]string{"edition": "1.35", "query": query})
	result, err := a.search(context.Background(), toolRequest(string(raw)))
	if err != nil || result.IsError {
		t.Fatal(result, err)
	}
	var v struct{ Arguments []string }
	if json.Unmarshal([]byte(resultText(result)), &v) != nil {
		t.Fatal(result)
	}
	args := v.Arguments
	if args[len(args)-1] != query || args[len(args)-2] != "kubernetes" || args[len(args)-3] != "--" {
		t.Fatal(args)
	}
	for _, raw := range []string{`{}`, `null`, `[]`, `{"edition":null,"query":"q"}`, `{"edition":"one","edition":"two","query":"q"}`, `{"edition":"one","query":"q","target":"other"}`, `{"edition":"one","query":1}`, `{"edition":"one","query":"\ud800"}`, `{"edition":"one","query":" "}`, `{"edition":"one","query":"\u0000"}`} {
		r, e := a.search(context.Background(), toolRequest(raw))
		if e != nil || !r.IsError {
			t.Fatal(raw, r, e)
		}
	}
	tooLong, _ := json.Marshal(map[string]string{"edition": "one", "query": strings.Repeat("é", 2049)})
	r, _ := a.search(context.Background(), toolRequest(string(tooLong)))
	if !r.IsError {
		t.Fatal("UTF-8 byte limit")
	}
	for _, raw := range []string{`{}`, `{"source_ref":"ref","scope":"bad"}`, `{"source_ref":"ref","scope":null}`, `{"source_ref":"ref","scope":""}`, `{"source_ref":"ref","edition":"two"}`} {
		r, _ := a.read(context.Background(), toolRequest(raw))
		if !r.IsError {
			t.Fatal(raw, r)
		}
	}
	r, _ = a.read(context.Background(), toolRequest(`{"source_ref":"ref"}`))
	if r.IsError || !strings.Contains(resultText(r), `"exact"`) {
		t.Fatal(r)
	}
}

func TestToolsFailureAndOverflowHaveNoPartialOutput(t *testing.T) {
	for _, mode := range []string{"failure", "overflow"} {
		t.Run(mode, func(t *testing.T) {
			observed := make(chan *exec.Cmd, 1)
			a := newTestTools(childCommand(mode, nil, observed))
			a.options.output = 128
			r, _ := a.search(context.Background(), toolRequest(`{"edition":"one","query":"q"}`))
			cmd := <-observed
			if !r.IsError || strings.Contains(resultText(r), "partial") || strings.Contains(resultText(r), "secret") || strings.Contains(resultText(r), "xxxx") {
				t.Fatal(r)
			}
			if cmd.ProcessState == nil {
				t.Fatal("child was not waited")
			}
		})
	}
}

func TestToolsCancellationJoinsChildAndBusyDoesNotQueue(t *testing.T) {
	for _, cancelBase := range []bool{false, true} {
		t.Run(map[bool]string{false: "request", true: "server"}[cancelBase], func(t *testing.T) {
			rd, wr, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer rd.Close()
			defer wr.Close()
			base, baseCancel := context.WithCancel(context.Background())
			defer baseCancel()
			request, requestCancel := context.WithCancel(context.Background())
			defer requestCancel()
			observed := make(chan *exec.Cmd, 2)
			a := newTestTools(childCommand("wait", []*os.File{wr}, observed))
			a.base = base
			done := make(chan *mcp.CallToolResult, 1)
			go func() { r, _ := a.search(request, toolRequest(`{"edition":"one","query":"q"}`)); done <- r }()
			var ready [1]byte
			if _, err = io.ReadFull(rd, ready[:]); err != nil {
				t.Fatal(err)
			}
			busy, _ := a.read(context.Background(), toolRequest(`{"source_ref":"ref"}`))
			if !busy.IsError || !strings.Contains(resultText(busy), "still running") {
				t.Fatal(busy)
			}
			if cancelBase {
				baseCancel()
			} else {
				requestCancel()
			}
			select {
			case r := <-done:
				if !r.IsError {
					t.Fatal(r)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("cancel failed to join")
			}
			cmd := <-observed
			if cmd.ProcessState == nil {
				t.Fatal("unwaited child")
			}
			if len(observed) != 0 {
				t.Fatal("busy request launched a child")
			}
			if a.busy.Load() {
				t.Fatal("admission retained")
			}
		})
	}
}

func TestToolsProtocolAndDisconnect(t *testing.T) {
	left, right := net.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- serveTools(ctx, left, left, toolsOptions{target: "kubernetes", limit: 5}, childCommand("success", nil, nil))
	}()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.IOTransport{Reader: right, Writer: right}, nil)
	if err != nil {
		t.Fatal(err)
	}
	listing, err := session.ListTools(ctx, nil)
	if err != nil || len(listing.Tools) != 2 {
		t.Fatal(listing, err)
	}
	r, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "search", Arguments: map[string]string{"edition": "one", "query": "q"}})
	if err != nil || r.IsError {
		t.Fatal(r, err)
	}
	if r.StructuredContent != nil {
		t.Fatal("duplicated evidence")
	}
	r, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "read", Arguments: map[string]string{"source_ref": "ref", "scope": "section"}})
	if err != nil || r.IsError {
		t.Fatal(r, err)
	}
	if err = session.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("disconnect did not stop server")
	}
}

type observedWriter struct {
	io.WriteCloser
	once    sync.Once
	started chan struct{}
}

func (w *observedWriter) Write(p []byte) (int, error) {
	w.once.Do(func() { close(w.started) })
	return w.WriteCloser.Write(p)
}
func TestToolsCancellationUnblocksUnreadOutput(t *testing.T) {
	input, feed := io.Pipe()
	defer feed.Close()
	drain, output := io.Pipe()
	defer drain.Close()
	writer := &observedWriter{WriteCloser: output, started: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- serveTools(ctx, input, writer, toolsOptions{target: "t", limit: 1}, childCommand("success", nil, nil))
	}()
	sent := make(chan error, 1)
	go func() {
		_, err := io.WriteString(feed, "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"initialize\",\"params\":{\"protocolVersion\":\"2025-06-18\",\"capabilities\":{},\"clientInfo\":{\"name\":\"test\",\"version\":\"1\"}}}\n")
		sent <- err
	}()
	select {
	case <-writer.started:
	case <-time.After(5 * time.Second):
		t.Fatal("no initialization response")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("unread output blocked shutdown")
	}
	<-sent
}

func TestToolsInputBounds(t *testing.T) {
	cases := map[string]string{
		"batch": "[]\n", "trailing": "{}{}\n", "broken": "{\n", "large": `{"x":"` + strings.Repeat("x", toolFrameBytes) + `"}` + "\n",
		"messages": strings.Repeat("{}\n", toolSessionMessages+1),
		"bytes":    strings.Repeat(`{"x":"`+strings.Repeat("x", 128<<10)+`"}`+"\n", 65),
	}
	for name, wire := range cases {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			input := newToolInput(io.NopCloser(strings.NewReader(wire)), cancel)
			_, err := io.Copy(io.Discard, input)
			if err == nil || !input.failed.Load() || ctx.Err() == nil {
				t.Fatal(err, input.failed.Load(), ctx.Err())
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	input := newToolInput(io.NopCloser(strings.NewReader("{}\n")), cancel)
	data, err := io.ReadAll(input)
	if err != nil || string(data) != "{}\n" || input.failed.Load() || ctx.Err() == nil {
		t.Fatal(string(data), err)
	}
}

func TestToolsDisconnectCancelsRunningChild(t *testing.T) {
	left, right := net.Pipe()
	rd, wr, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer rd.Close()
	defer wr.Close()
	observed := make(chan *exec.Cmd, 1)
	done := make(chan error, 1)
	go func() {
		done <- serveTools(context.Background(), left, left, toolsOptions{target: "t", limit: 1}, childCommand("wait", []*os.File{wr}, observed))
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.IOTransport{Reader: right, Writer: right}, nil)
	if err != nil {
		t.Fatal(err)
	}
	call := make(chan error, 1)
	go func() {
		_, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "search", Arguments: map[string]string{"edition": "one", "query": "q"}})
		call <- err
	}()
	var ready [1]byte
	if _, err = io.ReadFull(rd, ready[:]); err != nil {
		t.Fatal(err)
	}
	_ = right.Close()
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("EOF failed to stop child")
	}
	if cmd := <-observed; cmd.ProcessState == nil {
		t.Fatal("EOF returned before child joined")
	}
	select {
	case <-call:
	case <-ctx.Done():
		t.Fatal("client call did not stop")
	}
	_ = session.Close()
}

func TestToolsDeadlineAndRejectedArgumentsDoNotLaunch(t *testing.T) {
	observed := make(chan *exec.Cmd, 1)
	a := newTestTools(childCommand("success", nil, observed))
	r, _ := a.search(context.Background(), toolRequest(`{"edition":"one","query":"q","exec":"other"}`))
	if !r.IsError || len(observed) != 0 {
		t.Fatal("bad input launched command")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r, _ = a.search(ctx, toolRequest(`{"edition":"one","query":"q"}`))
	if !r.IsError || len(observed) != 0 {
		t.Fatal("canceled input launched command")
	}
}

func TestToolsHelpAndSettings(t *testing.T) {
	var out bytes.Buffer
	if err := runTools(context.Background(), []string{"--help"}, nil, &out); err != nil || !strings.Contains(out.String(), "256 JSON messages") {
		t.Fatal(out.String(), err)
	}
	for _, args := range [][]string{nil, {"--target", "t", "--limit", "0"}, {"--target", "t", "--max-output-bytes", "16777217"}, {"--target", "t", "--timeout", "6m"}, {"--target", "t", "--worker-socket", "sock"}} {
		out.Reset()
		if err := runTools(context.Background(), args, nil, &out); err == nil || out.Len() != 0 {
			t.Fatal(args, err, out.String())
		}
	}
}

func TestToolsSignalJoinsRunningChild(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("os.Interrupt signaling is unavailable on Windows")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	rd, wr, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer rd.Close()
	defer wr.Close()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestToolChild$")
	cmd.Env = append(os.Environ(), "CONCORDANCE_TOOL_TEST_CHILD=server")
	cmd.ExtraFiles = []*os.File{wr}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.IOTransport{Reader: stdout, Writer: stdin}, nil)
	if err != nil {
		t.Fatal(err)
	}
	call := make(chan error, 1)
	go func() {
		_, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "search", Arguments: map[string]string{"edition": "one", "query": "q"}})
		call <- err
	}()
	ready := make(chan error, 1)
	go func() { var b [1]byte; _, err := io.ReadFull(rd, b[:]); ready <- err }()
	select {
	case err = <-ready:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("child did not start")
	}
	if err = cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	select {
	case <-call:
	case <-ctx.Done():
		t.Fatal("SIGINT did not stop server")
	}
	_ = session.Close()
	if err = cmd.Wait(); err != nil {
		t.Fatal("server did not join child", err)
	}
}
