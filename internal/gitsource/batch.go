package gitsource

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
)

// Callers admit the complete selection's counts, modes and byte budgets first.
// There is one sequential process per selection, with no cross-call state/cache.
func captureBlobs(ctx context.Context, repository string, entries []treeEntry) ([]Document, error) {
	return captureBatch(ctx, gitCommand(ctx, repository, "cat-file", "--batch"), entries)
}

// The concrete command parameter also permits real-child fault tests. The command
// must be unstarted, created with CommandContext(ctx), and owned by this call.
// exec owns the stdin copier/context watcher; Wait joins them on every started path.
func captureBatch(ctx context.Context, cmd *exec.Cmd, entries []treeEntry) (out []Document, err error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, nil
	}
	var input strings.Builder
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		input.WriteString(entry.id)
		input.WriteByte('\n')
	}
	cmd.Stdin = strings.NewReader(input.String())
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var once sync.Once
	var closeErr error
	closeOutput := func() error {
		once.Do(func() { closeErr = pipe.Close() })
		return closeErr
	}
	// Killing Git alone need not close a pipe inherited by a descendant. Close our
	// reader on cancellation too, so a pending header/body/EOF read can finish.
	cmd.Cancel = func() error { return errors.Join(closeOutput(), cmd.Process.Kill()) }
	if err := cmd.Start(); err != nil {
		return nil, errors.Join(err, closeOutput(), ctx.Err())
	}
	defer func() {
		if err != nil {
			// A malformed response may leave Git blocked writing or reading. Kill it
			// before Wait; the owned input-copy goroutine must also be released.
			err = errors.Join(err, cmd.Process.Kill())
		}
		err = errors.Join(err, closeOutput(), cmd.Wait(), ctx.Err())
		if err != nil {
			out = nil
		}
	}()
	out, err = readBatch(ctx, pipe, entries)
	return out, err
}

func readBatch(ctx context.Context, input io.Reader, entries []treeEntry) ([]Document, error) {
	// The largest supported header is a 64-hex ID, " blob ", 19 decimal size
	// digits and LF (90 bytes). ReadSlice rejects a line exceeding this buffer.
	r := bufio.NewReaderSize(input, 128)
	out := make([]Document, 0, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		header, err := r.ReadSlice('\n')
		if err != nil || string(header) != fmt.Sprintf("%s blob %d\n", entry.id, entry.size) {
			return nil, errors.Join(ErrCorrupt, err)
		}
		// The size is from the already-admitted tree, never a response allocation.
		var body []byte
		if entry.size > 0 {
			body = make([]byte, int(entry.size))
		}
		if _, err := io.ReadFull(r, body); err != nil {
			return nil, errors.Join(ErrCorrupt, err)
		}
		separator, err := r.ReadByte()
		if err != nil || separator != '\n' {
			return nil, errors.Join(ErrCorrupt, err)
		}
		doc, err := verifiedBlob(entry, entry.name, body)
		if err != nil {
			return nil, err
		}
		out = append(out, doc)
	}
	if _, err := r.ReadByte(); err != io.EOF {
		return nil, errors.Join(ErrCorrupt, err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
