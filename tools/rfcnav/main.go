// Command rfcnav diagnoses exact navigation through an offline pinned TXT corpus.
// Its escaped text is for people, not a stable agent protocol. It neither queries
// relevance indexes nor imports RFCs into the application.
package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"time"

	"github.com/sidsharma96/concordance/tools/internal/textnav"
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
	f := flag.NewFlagSet("rfcnav", flag.ContinueOnError)
	rootPath := f.String("root", "", "directory containing retained rfcN.txt bodies")
	pinsPath := f.String("pins", "", "trusted existing rfcN sha256 inventory")
	exclude := f.String("exclude", "", "explicit comma-separated RFC IDs omitted from this open")
	op := f.String("op", "lookup", "lookup, find or read")
	id := f.String("id", "", "exact lowercase rfcN ID")
	literal := f.String("literal", "", "exact UTF-8 literal for find; case sensitive")
	from := f.Int("from", 0, "find start byte offset")
	limit := f.Int("limit", 8, "find page size, at most 16")
	start := f.Int("start", 0, "read start byte offset, inclusive")
	end := f.Int("end", 0, "read end byte offset, exclusive")
	timeout := f.Duration("timeout", 15*time.Second, "cooperative deadline; local reads are not preemptive")
	if err := f.Parse(args); err != nil {
		return err
	}
	if ctx == nil || out == nil || f.NArg() != 0 || *rootPath == "" || *pinsPath == "" || !rfcID(*id) || *timeout <= 0 || (*op != "lookup" && *op != "find" && *op != "read") {
		return errors.New("provide root, pins, exact RFC ID and lookup/find/read operation")
	}
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return err
	}
	pins, err := readPins(*pinsPath, *exclude)
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(*rootPath)
	if err != nil {
		return err
	}
	corpus, openErr := textnav.Open(ctx, root, pins, textnav.OpenLimits{MaxDocuments: 1000, MaxDocumentBytes: 8 << 20, MaxCorpusBytes: 64 << 20, MaxMetadataBytes: 1 << 20})
	if err := errors.Join(openErr, root.Close()); err != nil {
		return err
	}
	s, err := corpus.NewSession(textnav.Limits{MaxCalls: 32, MaxReturnedBytes: 16 << 10, MaxLiteralBytes: 256, MaxFindMatches: 16})
	if err != nil {
		return err
	}
	doc, err := s.Lookup(ctx, *id)
	if err != nil {
		return err
	}
	// Assemble before writing so later navigation/validation errors emit no
	// success diagnostics. Writer errors can still leave partial output.
	var b bytes.Buffer
	info := corpus.Inventory()
	fmt.Fprintf(&b, "corpus documents=%d verified-bytes=%d\n", info.Documents, info.Bytes)
	fmt.Fprintf(&b, "document id=%s sha256=%x bytes=%d\n", doc.Ref.ID, doc.Ref.SHA256, doc.Bytes)
	switch *op {
	case "find":
		page, err := s.Find(ctx, doc.Ref, *literal, *from, *limit)
		if err != nil {
			return err
		}
		for _, match := range page.Matches {
			fmt.Fprintf(&b, "match [%d,%d)\n", match.Start, match.End)
		}
		fmt.Fprintf(&b, "more=%t next=%d\n", page.More, page.Next)
	case "read":
		part, err := s.Read(ctx, doc.Ref, textnav.Span{Start: *start, End: *end})
		if err != nil {
			return err
		}
		fmt.Fprintf(&b, "excerpt [%d,%d) text=%q\n", part.Span.Start, part.Span.End, part.Text)
	}
	stats := s.Stats()
	fmt.Fprintf(&b, "session calls=%d returned-source-bytes=%d\n", stats.Calls, stats.ReturnedBytes)
	n, err := out.Write(b.Bytes())
	if err == nil && n != b.Len() {
		err = io.ErrShortWrite
	}
	return err
}
