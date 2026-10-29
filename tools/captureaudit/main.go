// Command captureaudit checks a trusted prepared capture without Git or storage I/O.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/sidsharma96/concordance/internal/capturefiles"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "captureaudit:", err)
		os.Exit(1)
	}
}
func run(ctx context.Context, args []string, out io.Writer) error {
	f := flag.NewFlagSet("captureaudit", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	directory := f.String("directory", "", "trusted stable capture directory containing capture.json")
	maxDescriptor := f.Int("max-description-bytes", 4<<20, "maximum encoded descriptor bytes")
	maxFile := f.Int64("max-file-bytes", 8<<20, "maximum bytes per captured HTML/recipe file")
	maxPayload := f.Int64("max-payload-bytes", 128<<20, "maximum payload bytes counted per reference occurrence")
	timeout := f.Duration("timeout", 15*time.Second, "cooperative deadline; cannot preempt blocked filesystem I/O")
	if err := f.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			_, err = io.WriteString(out, "Usage: go run ./tools/captureaudit -directory CAPTURE\nChecks capture.json and retained file hashes. No Git, database, object store or renderer is accessed.\n")
			return err
		}
		return errors.New("invalid captureaudit flags")
	}
	if f.NArg() != 0 || *directory == "" || *timeout <= 0 || ctx == nil {
		return errors.New("provide -directory and positive limits/timeout")
	}
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	editions, err := capturefiles.ReadDirectory(ctx, *directory, capturefiles.Limits{MaxDescriptorBytes: *maxDescriptor, MaxEditions: 32, MaxCaptures: 128, MaxPages: 10000, MaxDependencies: 100000, MaxFiles: 10128, MaxStringBytes: 8192, MaxValueBytes: 4 << 20, MaxFileBytes: *maxFile, MaxPayloadBytes: *maxPayload})
	if err != nil {
		return err
	}
	captures, pages, dependencies := 0, 0, 0
	var payload int64
	for _, e := range editions {
		for _, c := range e.Captures {
			captures++
			dependencies += len(c.Dependencies)
			payload += int64(len(c.Recipe))
			for _, p := range c.Pages {
				pages++
				payload += int64(len(p.HTML))
			}
		}
	}
	var report strings.Builder
	fmt.Fprintf(&report, "format\t%s\neditions\t%d\ncaptures\t%d\npages\t%d\ndeclared_git_dependencies\t%d\npayload_reference_bytes\t%d\nfile_checks\tPASS\n", capturefiles.Format, len(editions), captures, pages, dependencies, payload)
	_, err = io.WriteString(out, report.String())
	return err
}
