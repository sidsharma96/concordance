package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"
)

func inspectionFixture(t *testing.T) (string, []string) {
	t.Helper()
	body := []byte("CNDX\x00\x01\x05empty\x16concordance.article/v1\x0ehtmlpassage/v1\x0chtmlterms/v1\x1btechnical/v1/unicode-17.0.0\x00\x00\x00\x00\x00")
	file := filepath.Join(t.TempDir(), "edition.bin")
	if err := os.WriteFile(file, body, 0600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(body)
	return file, []string{"-file", file, "-edition", "empty", "-sha256", hex.EncodeToString(digest[:]), "-bytes", strconv.Itoa(len(body))}
}
func TestInspectValidatesBeforeWriting(t *testing.T) {
	file, args := inspectionFixture(t)
	var out bytes.Buffer
	if err := run(t.Context(), args, &out); err != nil || !json.Valid(out.Bytes()) {
		t.Fatal(out.String(), err)
	}
	var v struct {
		Edition  string
		Articles []any
		Passages []any
		Words    []any
	}
	if err := json.Unmarshal(out.Bytes(), &v); err != nil || v.Edition != "empty" || v.Articles == nil || v.Passages == nil || v.Words == nil {
		t.Fatal(v, err)
	}
	for _, extra := range [][]string{{"-edition", "wrong"}, {"-sha256", "00"}, {"-bytes", "1"}, {"-max-input-bytes", "10"}, {"-max-output-bytes", "1"}, {"-timeout", "0s"}, {"-file", filepath.Dir(file)}, {"-max-input-bytes", strconv.Itoa(int(^uint(0) >> 1))}, {"unexpected"}} {
		out.Reset()
		a := append(append([]string{}, args...), extra...)
		if err := run(t.Context(), a, &out); err == nil || out.Len() != 0 {
			t.Fatal(extra, err, out.String())
		}
	}
	if err := run(t.Context(), nil, &out); err == nil {
		t.Fatal("missing arguments")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	out.Reset()
	if err := run(ctx, args, &out); !errors.Is(err, context.Canceled) || out.Len() != 0 {
		t.Fatal(err)
	}
	if err := run(t.Context(), args, shortWriter{}); !errors.Is(err, io.ErrShortWrite) {
		t.Fatal("short write", err)
	}
	sentinel := errors.New("write failed")
	if err := run(t.Context(), args, failedWriter{sentinel}); !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
}

type shortWriter struct{}

func (shortWriter) Write(b []byte) (int, error) { return len(b) - 1, nil }

type failedWriter struct{ err error }

func (w failedWriter) Write([]byte) (int, error) { return 0, w.err }

func TestRejectFIFOWithoutOpeningIt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX FIFO fixture")
	}
	command, err := exec.LookPath("mkfifo")
	if err != nil {
		t.Skip("mkfifo unavailable")
	}
	fifo := filepath.Join(t.TempDir(), "pipe")
	if err := exec.Command(command, fifo).Run(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := readFile(fifo, 1024); done <- err }()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("accepted FIFO")
		}
	case <-ctx.Done():
		// Unblock a regressed Open so the test does not leak its reader goroutine.
		endpoint, err := os.OpenFile(fifo, os.O_RDWR, 0600)
		if err != nil {
			t.Fatal(err)
		}
		if err := endpoint.Close(); err != nil {
			t.Fatal(err)
		}
		<-done
		t.Fatal("opening a FIFO waited for an endpoint before rejecting it")
	}
}
