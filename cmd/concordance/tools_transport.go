package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync/atomic"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	toolFrameBytes      = 256 << 10
	toolSessionBytes    = 8 << 20
	toolSessionMessages = 256
)

// toolInput admits complete single-object JSON lines before the SDK's decoder.
// Session totals bound cumulative SDK queued input, not just active CLI work.
// pending owns one admitted line and its delimiter until the SDK consumes it.
type toolInput struct {
	input           io.ReadCloser
	scanner         *bufio.Scanner
	cancel          context.CancelFunc
	pending         []byte
	bytes, messages int
	failed          atomic.Bool
}

func newToolInput(in io.ReadCloser, cancel context.CancelFunc) *toolInput {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 4096), toolFrameBytes+2)
	return &toolInput{input: in, scanner: scanner, cancel: cancel}
}
func (r *toolInput) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if len(r.pending) == 0 {
		if !r.scanner.Scan() {
			err := r.scanner.Err()
			if err != nil {
				r.failed.Store(true)
			} else {
				err = io.EOF
			}
			r.cancel()
			return 0, err
		}
		line := r.scanner.Bytes()
		r.messages++
		if len(line) > toolFrameBytes || len(line) > toolSessionBytes-r.bytes || r.messages > toolSessionMessages || !utf8.Valid(line) || !json.Valid(line) || !bytes.HasPrefix(bytes.TrimSpace(line), []byte("{")) {
			r.failed.Store(true)
			r.cancel()
			return 0, errors.New("tool input frame or session limit exceeded")
		}
		r.bytes += len(line)
		// The decoder needs the frame delimiter; copy once to append it without
		// modifying the scanner's borrowed backing array.
		r.pending = append(append([]byte(nil), line...), '\n')
	}
	n := copy(p, r.pending)
	r.pending = r.pending[n:]
	return n, nil
}
func (r *toolInput) Close() error { return r.input.Close() }

// serveTools owns these streams. The pinned SDK detaches handler contexts from
// server cancellation and drains handlers before closing its connection. Close
// our streams independently so a dead client cannot hold that drain on stdout.
func serveTools(parent context.Context, in io.ReadCloser, out io.WriteCloser, o toolsOptions, command toolCommand) error {
	base, cancel := context.WithCancel(parent)
	defer cancel()
	input := newToolInput(in, cancel)
	closed := make(chan struct{})
	context.AfterFunc(base, func() {
		_ = in.Close()
		_ = out.Close()
		close(closed)
	})
	server := newToolServer(&agentTools{base: base, options: o, command: command})
	err := server.Run(base, &mcp.IOTransport{Reader: input, Writer: out})
	cancel()
	<-closed // Join our close callback; SDK Run already waited for its handlers.
	if parent.Err() != nil {
		return commandFailure("tool server stopped", parent.Err())
	}
	if input.failed.Load() {
		return errors.New("tool input failed or exceeded session limits")
	}
	if err != nil && !errors.Is(err, context.Canceled) {
		return errors.New("tool protocol session failed")
	}
	return nil
}
