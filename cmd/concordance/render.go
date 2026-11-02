package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/sidsharma96/concordance/internal/catalog"
	"github.com/sidsharma96/concordance/internal/publishedsearch"
)

// The buffer bounds admitted display bytes. append/fmt and string escaping still
// have ordinary allocation capacity/scratch costs; this is not a heap quota.
type displayBuffer struct {
	body []byte
	max  int
}

func (b *displayBuffer) Write(p []byte) (int, error) {
	if len(p) > b.max-len(b.body) {
		return 0, errOutputLimit
	}
	b.body = append(b.body, p...)
	return len(p), nil
}
func visible(s string) string { q := strconv.Quote(s); return q[1 : len(q)-1] }

func renderSearch(ctx context.Context, r *publishedsearch.Reader, name string, selection publishedsearch.Selection, batch publishedsearch.QuotedResult, maxBytes int) ([]byte, error) {
	b := displayBuffer{max: maxBytes}
	if _, err := fmt.Fprintf(&b, "Searching %q | edition %q\nCollection %q | generation %q\nShowing %d of %d indexed matches\n", name, selection.Edition, selection.Collection, selection.Generation, len(batch.Hits), batch.TotalMatches); err != nil {
		return nil, err
	}
	for i, hit := range batch.Hits {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		loc, err := r.Location(ctx, hit.Ref)
		if err != nil {
			return nil, err
		}
		if _, err := fmt.Fprintf(&b, "\n%d. %q:%q\n   revision %q | retained blocks [%d,%d)\n", i+1, loc.Document.Source.ID, loc.Document.NativeID, loc.Document.Revision, loc.Body.First, loc.Body.After); err != nil {
			return nil, err
		}
		for rest := hit.Quote; ; {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			line, next, more := strings.Cut(rest, "\n")
			if _, err := fmt.Fprintf(&b, "   %s\n", visible(line)); err != nil {
				return nil, err
			}
			if !more {
				break
			}
			rest = next
		}
	}
	return b.body, nil
}
func renderTarget(t catalog.Target) ([]byte, error) {
	b := displayBuffer{max: 32 << 10}
	_, err := fmt.Fprintf(&b, "Target %q | revision %d\nCollection %q | generation %q | default edition %q\n", t.Name, t.Revision, t.Value.Collection, t.Value.Generation, t.Value.DefaultEdition)
	if err != nil {
		return nil, err
	}
	return b.body, nil
}
