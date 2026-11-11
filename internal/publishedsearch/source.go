package publishedsearch

import (
	"context"
	"strings"

	"github.com/sidsharma96/concordance/internal/htmlarticle"
	"github.com/sidsharma96/concordance/internal/htmlpassage"
	"github.com/sidsharma96/concordance/internal/objectstore"
)

// SourceReference is a value locator, not a reader handle or permission. Slot is
// edition-local and meaningful only in Selection's exact published generation.
// Source identity and expected bytes must match that slot. Body may span complete
// canonical passages after navigation. Callers authorize every use before Open.
type SourceReference struct {
	Selection          Selection
	Slot               int
	SourceID, NativeID string
	ArticleSHA256      [32]byte
	Body               htmlpassage.Range
}

// SourceReference turns a checked reader handle into a re-resolvable locator.
// It performs no I/O; publishing a newer generation cannot redirect this value.
func (r *Reader) SourceReference(ctx context.Context, ref Reference) (SourceReference, error) {
	loc, err := r.Location(ctx, ref)
	if err != nil {
		return SourceReference{}, err
	}
	pos, err := r.data.edition.Location(ref.passage)
	if err != nil {
		return SourceReference{}, err
	}
	return SourceReference{loc.Selection, pos.Article, loc.Document.Source.ID, loc.Document.NativeID, loc.Document.Article.SHA256, loc.Body}, nil
}

// SourceNotice preserves a label's relationship to its retained block range.
// Labels are context, not bytes silently inserted into the verbatim body.
type SourceNotice struct {
	Body  htmlpassage.Range
	Label string
}

// SourceRead owns text and mutable slices. Navigation references remain in the
// same exact article; nil means the direction is unavailable. No body cache stays
// reachable after return. Location owns its copied manifest metadata.
type SourceRead struct {
	Location               Location
	Reference              SourceReference
	Quote                  string
	Headings               []string
	Notices                []SourceNotice
	Previous, Next, Parent *SourceReference
}

// ReadSource resolves only this reader's exact published selection. It validates
// identity/index/hash and declared article bytes before one strict article GET,
// then verifies provenance and rebuilds canonical ranges for navigation. A small
// returned section still reads the entire article. Work is bounded by existing
// manifest resolution and article/plan limits, plus selected output bytes. No
// ranking, current-target lookup, permission token, retry or partial result.
func (r *Reader) ReadSource(ctx context.Context, ref SourceReference, scope htmlpassage.Scope, l EvidenceLimits) (SourceRead, error) {
	if err := ready(ctx); err != nil {
		return SourceRead{}, err
	}
	if r == nil || r.data == nil || !validEvidenceLimits(l) || ref.Slot < 0 || ref.Body.First < 0 || ref.Body.After <= ref.Body.First {
		return SourceRead{}, ErrInvalid
	}
	if scope != htmlpassage.Exact && scope != htmlpassage.Section && scope != htmlpassage.Parent && scope != htmlpassage.Previous && scope != htmlpassage.Next {
		return SourceRead{}, ErrInvalid
	}
	if ref.Selection != r.data.selection {
		return SourceRead{}, ErrMismatch
	}
	doc, err := r.data.manifest.Resolve(ctx, ref.Selection.Edition, ref.Slot)
	if err != nil {
		return SourceRead{}, err
	}
	indexed, err := r.data.edition.Article(ref.Slot)
	if err != nil {
		return SourceRead{}, err
	}
	if ref.SourceID != doc.Source.ID || ref.NativeID != doc.NativeID || ref.ArticleSHA256 != doc.Article.SHA256 || indexed.Object.SHA256 != doc.Article.SHA256 || int64(indexed.Object.Bytes) != doc.Article.Bytes || ref.Body.After > indexed.Blocks {
		return SourceRead{}, ErrMismatch
	}
	if doc.Article.Bytes > int64(r.data.article.MaxEncodedBytes) {
		return SourceRead{}, ErrLimit
	}
	wire, err := r.data.objects.Read(ctx, objectstore.Ref{SHA256: doc.Article.SHA256, Bytes: doc.Article.Bytes})
	if err != nil {
		return SourceRead{}, err
	}
	a, err := r.data.manifest.VerifyArticle(ctx, ref.Selection.Edition, ref.Slot, wire, r.data.article)
	if err != nil {
		return SourceRead{}, err
	}
	if len(a.Blocks) != indexed.Blocks {
		return SourceRead{}, ErrMismatch
	}
	nav, err := htmlpassage.Navigate(ctx, a, ref.Body, scope, l.Passages)
	if err != nil {
		return SourceRead{}, err
	}
	ref.Body = nav.Body
	out := SourceRead{Location: Location{ref.Selection, doc, nav.Body}, Reference: ref}
	neighbor := func(body htmlpassage.Range) *SourceReference {
		if body.After == 0 {
			return nil
		}
		v := ref
		v.Body = body
		return &v
	}
	out.Previous, out.Next, out.Parent = neighbor(nav.Previous), neighbor(nav.Next), neighbor(nav.Parent)
	remaining := l.MaxQuoteBytes
	for i := nav.Body.First; i < nav.Body.After; i++ {
		if err := ctx.Err(); err != nil {
			return SourceRead{}, err
		}
		if i > nav.Body.First {
			if remaining == 0 {
				return SourceRead{}, ErrLimit
			}
			remaining--
		}
		if len(a.Blocks[i].Text) > remaining {
			return SourceRead{}, ErrLimit
		}
		remaining -= len(a.Blocks[i].Text)
	}
	var text strings.Builder
	text.Grow(l.MaxQuoteBytes - remaining)
	for i := nav.Body.First; i < nav.Body.After; i++ {
		if err := ctx.Err(); err != nil {
			return SourceRead{}, err
		}
		if i > nav.Body.First {
			text.WriteByte('\n')
		}
		text.WriteString(a.Blocks[i].Text)
	}
	out.Quote = text.String()
	remaining = l.MaxContextBytes
	for _, i := range nav.Headings {
		text := a.Blocks[i].Text
		if len(text) > remaining {
			return SourceRead{}, ErrLimit
		}
		remaining -= len(text)
		out.Headings = append(out.Headings, text)
	}
	for _, g := range a.Groups {
		if err := ctx.Err(); err != nil {
			return SourceRead{}, err
		}
		// Empty labels at a boundary follow the next passage, except at EOF.
		overlaps := g.First < nav.Body.After && g.After > nav.Body.First
		if g.First == g.After {
			overlaps = g.First >= nav.Body.First && (g.First < nav.Body.After || g.First == len(a.Blocks) && nav.Body.After == len(a.Blocks))
		}
		if g.Kind != htmlarticle.Notice || g.Label == "" || !overlaps {
			continue
		}
		if len(g.Label) > remaining {
			return SourceRead{}, ErrLimit
		}
		remaining -= len(g.Label)
		out.Notices = append(out.Notices, SourceNotice{htmlpassage.Range{First: g.First, After: g.After}, g.Label})
	}
	if err := ctx.Err(); err != nil {
		return SourceRead{}, err
	}
	return out, nil
}
