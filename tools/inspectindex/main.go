// Command inspectindex verifies a local binary HTML edition file against an
// independently supplied reference and prints a bounded JSON diagnostic. It does
// not establish publisher authority, fetch objects or inspect a complete generation.
package main

import (
	"context"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"time"

	"github.com/sidsharma96/concordance/internal/htmledition"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run(ctx context.Context, args []string, out io.Writer) error {
	f := flag.NewFlagSet("inspectindex", flag.ContinueOnError)
	file := f.String("file", "", "stable local regular binary HTML edition file")
	edition := f.String("edition", "", "expected exact edition key")
	digest := f.String("sha256", "", "expected SHA-256, 64 hexadecimal digits")
	size := f.Int("bytes", 0, "expected exact file length")
	maxInput := f.Int("max-input-bytes", 16<<20, "encoded input byte ceiling")
	maxOutput := f.Int("max-output-bytes", 64<<20, "JSON diagnostic byte ceiling")
	timeout := f.Duration("timeout", 15*time.Second, "deadline; regular file reads are not preemptive")
	if err := f.Parse(args); err != nil {
		return err
	}
	if ctx == nil || f.NArg() != 0 || *file == "" || *edition == "" || *size <= 0 || *maxInput <= 0 || *maxInput == int(^uint(0)>>1) || *maxOutput <= 0 || *timeout <= 0 {
		return errors.New("provide file, edition, expected sha256/bytes and positive inspection limits")
	}
	hash, err := hex.DecodeString(*digest)
	if err != nil || len(hash) != 32 {
		return errors.New("provide a 64-digit expected SHA-256")
	}
	if *size > *maxInput {
		return htmledition.ErrLimit
	}
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return err
	}
	body, err := readFile(*file, *maxInput)
	if err != nil {
		return err
	}
	expected := htmledition.Ref{Bytes: *size}
	copy(expected.SHA256[:], hash)
	// These are explicit diagnostic budgets, not production capacity promises. The
	// decoder also admits record counts against minimum remaining encoded bytes.
	l := htmledition.Limits{MaxEncodedBytes: *maxInput, MaxArticles: 10000, MaxArticleBytes: 1 << 30, MaxBlocks: 1000000, MaxPassages: 100000, MaxTerms: 10000000, MaxPreparedBytes: 256 << 20, MaxKeyBytes: 16 << 20, MaxTermBytes: 64 << 10, MaxVocabulary: 100000, MaxPostings: 1000000}
	e, err := htmledition.Open(ctx, body, expected, *edition, l)
	if err != nil {
		return err
	}
	view, err := htmledition.Inspect(ctx, e, l, *maxOutput)
	if err != nil {
		return err
	}
	n, err := out.Write(view)
	if err == nil && n != len(view) {
		err = io.ErrShortWrite
	}
	return err
}
func readFile(path string, maxBytes int) ([]byte, error) {
	// Reject a known FIFO/device before Open can wait for another endpoint. The
	// caller keeps this local path stable: preflight Stat is not a race-free open
	// guarantee. Recheck the opened handle, and still verify its actual bytes later.
	before, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() {
		return nil, errors.New("inspect a regular file")
	}
	if before.Size() > int64(maxBytes) {
		return nil, htmledition.ErrLimit
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	info, statErr := f.Stat()
	if statErr != nil {
		return nil, errors.Join(statErr, f.Close())
	}
	if !info.Mode().IsRegular() {
		return nil, errors.Join(errors.New("inspect a regular file"), f.Close())
	}
	if info.Size() > int64(maxBytes) {
		return nil, errors.Join(htmledition.ErrLimit, f.Close())
	}
	body, readErr := io.ReadAll(io.LimitReader(f, int64(maxBytes)+1))
	closeErr := f.Close()
	if readErr != nil || closeErr != nil {
		return nil, errors.Join(readErr, closeErr)
	}
	if len(body) > maxBytes {
		return nil, htmledition.ErrLimit
	}
	return body, nil
}
