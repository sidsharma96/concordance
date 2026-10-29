package gitsource

import (
	"bytes"
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func batchEntry(name, body string) treeEntry {
	h := sha1.New()
	fmt.Fprintf(h, "blob %d\x00", len(body))
	h.Write([]byte(body))
	return treeEntry{name: name, id: fmt.Sprintf("%x", h.Sum(nil)), mode: "100644", size: int64(len(body))}
}
func batchWire(entry treeEntry, body string) string {
	return fmt.Sprintf("%s blob %d\n%s\n", entry.id, entry.size, body)
}

func TestBatchMatchesSingleBlobOracle(t *testing.T) {
	for _, format := range []string{"sha1", "sha256"} {
		t.Run(format, func(t *testing.T) {
			f := newFixture(t, format)
			for name, body := range map[string]string{"a.md": "same\x00\xff\n", "b.md": "same\x00\xff\n", "c.md": "", "d.bin": "raw\r\nbytes"} {
				f.write(t, name, body)
			}
			pin := f.commit(t)
			files, err := CaptureFiles(f.ctx, FilesRequest{Repository: f.repo, Commit: pin, Paths: []string{"d.bin", "c.md", "b.md", "a.md"}}, fileBudgets())
			if err != nil {
				t.Fatal(err)
			}
			for _, got := range files.Files {
				// The previous transport is an independent per-object process oracle.
				body, err := run(f.ctx, f.repo, int64(len(got.Body)), "cat-file", "blob", got.BlobID)
				if err != nil || !matchesBlob(body, got.BlobID) {
					t.Fatalf("single-object oracle: %v", err)
				}
				want := Document{Path: got.Path, BlobID: got.BlobID, Body: body, SHA256: sha256.Sum256(body)}
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("batch differs from single-object oracle at %q", got.Path)
				}
			}
			files.Files[0].Body[0] = 'X'
			if string(files.Files[1].Body) != "same\x00\xff\n" {
				t.Fatal("same-blob paths share mutable buffers")
			}
			t.Log("batch and single-object bytes match")
		})
	}
}

func TestReadBatchRejectsIncompleteOrCorruptOutput(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	a, b := batchEntry("a", "data"), batchEntry("b", "")
	valid := batchWire(a, "data") + batchWire(b, "")
	for _, tc := range []struct {
		name, wire string
	}{
		{"missing header", ""},
		{"missing object", a.id + " missing\n"},
		{"wrong ID", strings.Repeat("0", 40) + " blob 4\ndata\n"},
		{"wrong type", a.id + " tree 4\ndata\n"},
		{"wrong size", a.id + " blob 5\ndata\n"},
		{"nonminimal size", a.id + " blob 04\ndata\n"},
		{"oversized numeric header", a.id + " blob " + strings.Repeat("9", 1000) + "\n"},
		{"unterminated header", a.id + " blob 4"},
		{"truncated body", a.id + " blob 4\nda"},
		{"same-sized corrupt body", batchWire(a, "date") + batchWire(b, "")},
		{"wrong separator", a.id + " blob 4\ndataX"},
		{"missing second object", batchWire(a, "data")},
		{"truncated last separator", strings.TrimSuffix(valid, "\n")},
		{"extra object", valid + batchWire(b, "")},
		{"trailing byte", valid + "x"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := readBatch(ctx, strings.NewReader(tc.wire), []treeEntry{a, b})
			if !errors.Is(err, ErrCorrupt) || got != nil {
				t.Fatalf("partial or incorrect error: %v %v", got, err)
			}
		})
	}
	got, err := readBatch(ctx, strings.NewReader(valid), []treeEntry{a, b})
	if err != nil || len(got) != 2 || string(got[0].Body) != "data" || len(got[1].Body) != 0 {
		t.Fatalf("valid framing: %v %v", got, err)
	}
	lfs := "version https://git-lfs.github.com/spec/v1\n"
	l := batchEntry("pointer", lfs)
	if got, err := readBatch(ctx, strings.NewReader(batchWire(l, lfs)), []treeEntry{l}); !errors.Is(err, ErrUnsupported) || got != nil {
		t.Fatal("LFS accepted", err)
	}
}

func batchChild(ctx context.Context, mode string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestBatchChild$")
	cmd.Env = append(os.Environ(), "CONCORDANCE_BATCH_CHILD="+mode)
	cmd.Stderr = io.Discard
	cmd.WaitDelay = time.Second
	return cmd
}

// A real Go child supplies protocol/exit faults, without depending on shell tools.
func TestBatchChild(t *testing.T) {
	mode := os.Getenv("CONCORDANCE_BATCH_CHILD")
	if mode == "" {
		return
	}
	a := batchEntry("a", "data")
	switch mode {
	case "exit":
		fmt.Print(batchWire(a, "data"))
		os.Exit(7)
	case "corrupt":
		fmt.Print("unexpected\n")
		os.Exit(0)
	case "corrupt-blocked":
		fmt.Print("unexpected\n")
		block := os.NewFile(3, "block")
		_, _ = io.Copy(io.Discard, block)
		os.Exit(2)
	case "blocked-output", "blocked-input", "blocked-exit":
		// Signal the exact waiting boundary through fd3; fd4 stays open in the
		// parent until cleanup. No sleep establishes cancellation ordering.
		if mode == "blocked-output" {
			if _, err := io.Copy(io.Discard, os.Stdin); err != nil {
				os.Exit(2)
			}
		}
		if mode == "blocked-exit" {
			fmt.Print(batchWire(a, "data"))
			if err := os.Stdout.Close(); err != nil {
				os.Exit(2)
			}
		}
		ready := os.NewFile(3, "ready")
		if _, err := ready.Write([]byte{1}); err != nil {
			os.Exit(2)
		}
		if err := ready.Close(); err != nil {
			os.Exit(2)
		}
		block := os.NewFile(4, "block")
		_, err := io.Copy(io.Discard, block)
		if errors.Join(err, block.Close()) != nil {
			os.Exit(2)
		}
		os.Exit(0)
	default:
		os.Exit(2)
	}
}

func TestBatchProcessErrorsReturnNoDocumentsAndWait(t *testing.T) {
	for _, mode := range []string{"exit", "corrupt", "corrupt-blocked"} {
		t.Run(mode, func(t *testing.T) {
			if mode == "corrupt-blocked" && runtime.GOOS == "windows" {
				t.Skip("extra-file synchronization is unsupported on Windows")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			cmd := batchChild(ctx, mode)
			if mode == "corrupt-blocked" {
				r, w, err := os.Pipe()
				if err != nil {
					t.Fatal(err)
				}
				defer r.Close()
				defer w.Close()
				cmd.ExtraFiles = []*os.File{r}
			}
			got, err := captureBatch(ctx, cmd, []treeEntry{batchEntry("a", "data")})
			if err == nil || got != nil || cmd.ProcessState == nil {
				t.Fatalf("not failed and reaped: %v %v %v", got, err, cmd.ProcessState)
			}
			if mode != "exit" && (!errors.Is(err, ErrCorrupt) || ctx.Err() != nil) {
				t.Fatal("corruption cause lost", err)
			}
		})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, t.TempDir()+"/absent")
	if got, err := captureBatch(ctx, cmd, []treeEntry{batchEntry("a", "data")}); err == nil || got != nil || cmd.ProcessState != nil {
		t.Fatal("start failure accepted", err)
	}
	cmd = batchChild(ctx, "corrupt")
	if got, err := captureBatch(ctx, cmd, nil); err != nil || got != nil || cmd.Process != nil {
		t.Fatal("empty selection started a process", err)
	}
}

func TestBatchCancellationJoinsBlockedInputAndOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("extra-file synchronization is unsupported on Windows")
	}
	for _, mode := range []string{"blocked-output", "blocked-input", "blocked-exit"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			readyR, readyW, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer readyR.Close()
			defer readyW.Close()
			blockR, blockW, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer blockR.Close()
			defer blockW.Close()
			cmd := batchChild(ctx, mode)
			cmd.ExtraFiles = []*os.File{readyW, blockR}
			entries := []treeEntry{batchEntry("a", "data")}
			if mode == "blocked-input" {
				entries = make([]treeEntry, 10000) // larger than a pipe's input buffer
				for i := range entries {
					entries[i] = batchEntry("a", "data")
				}
			}
			done := make(chan error, 1)
			joined := false
			defer func() {
				cancel()
				if !joined {
					<-done
				}
			}()
			go func() {
				got, err := captureBatch(ctx, cmd, entries)
				if got != nil || cmd.ProcessState == nil {
					err = fmt.Errorf("partial result or unjoined child: %v", err)
				}
				done <- err
			}()
			if err := readyR.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
				t.Fatal(err)
			}
			var marker [1]byte
			if _, err := io.ReadFull(readyR, marker[:]); err != nil {
				cancel()
				<-done
				joined = true
				t.Fatal("waiting boundary not reached", err)
			}
			cancel()
			err = <-done
			joined = true
			if !errors.Is(err, context.Canceled) {
				t.Fatal("cancellation cause lost", err)
			}
		})
	}
}

func FuzzReadBatch(f *testing.F) {
	e := batchEntry("a", "data")
	canonical := batchWire(e, "data")
	f.Add([]byte(canonical))
	f.Add([]byte(""))
	f.Add([]byte(strings.Repeat("x", 129)))
	f.Fuzz(func(t *testing.T, wire []byte) {
		if len(wire) > 4096 {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		got, err := readBatch(ctx, bytes.NewReader(wire), []treeEntry{e})
		if err != nil {
			if got != nil {
				t.Fatal("partial decoder result")
			}
			return
		}
		if string(wire) != canonical || len(got) != 1 || string(got[0].Body) != "data" || got[0].SHA256 != sha256.Sum256([]byte("data")) {
			t.Fatal("accepted noncanonical or corrupt data")
		}
	})
}
