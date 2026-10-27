// Package htmlpassage forms bounded, structure-first passage ranges in a retained
// htmlarticle.Article. It does not copy text, tokenize, rank, persist or publish.
package htmlpassage

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/sidsharma96/concordance/internal/htmlarticle"
)

const Processor = "htmlpassage/v1"

var (
	ErrInvalid     = errors.New("invalid HTML passage input")
	ErrLimit       = errors.New("HTML passage limit exceeded")
	ErrUnsupported = errors.New("unsupported HTML passage input")
)

// Limits bound admitted work and returned metadata, not exact heap usage.
// All fields must be positive. Input bytes count every article string field.
// Body bytes count block Text joined with one LF between blocks. Context bytes
// count referenced heading Text and group Label, repeated for each reference.
// References count all heading and group indices across the entire plan.
type Limits struct {
	MaxBlocks       int
	MaxGroups       int
	MaxInputBytes   int
	MaxPassages     int
	MaxBodyBytes    int
	MaxReferences   int
	MaxContextBytes int
}

// Range is a nonempty half-open block interval [First, After).
type Range struct{ First, After int }

// Passage selects its body and context in the caller's unchanged Article.
// Headings are ancestor block indices strictly before Body.First that remain
// in scope throughout the body, after accounting for its own headings. Groups are
// indices into Article.Groups, including nested containers and empty groups.
// An empty group at a body boundary belongs to the next body, or the final body
// at EOF. These references preserve metadata; they do not choose scoring weights
// or make group labels part of a verbatim source quotation.
type Passage struct {
	Body     Range
	Headings []int
	Groups   []int
}

// Plan owns its ranges and reference slices, but not an Article. Its indices have
// meaning only with the exact unchanged article supplied to Build. The caller
// keeps that association and the article alive for all use; this is not a codec,
// content identity or authorization boundary. Processor versions the grouping rule.
type Plan struct {
	Processor string
	Passages  []Passage
}

// Build borrows article until return. The caller must not mutate its blocks,
// groups or strings during the call or while interpreting the returned indices.
// A deadline is required; cancellation is cooperative and no goroutines start.
// Invalid input, excessive work and oversized units return a zero Plan. No unit
// is automatically split or omitted. Admission limits belong to the caller.
func Build(ctx context.Context, article htmlarticle.Article, limits Limits) (Plan, error) {
	if err := ctx.Err(); err != nil {
		return Plan{}, err
	}
	if !validLimits(ctx, limits) {
		return Plan{}, ErrInvalid
	}
	outerAfter, err := validate(ctx, article, limits)
	if err != nil {
		return Plan{}, err
	}
	return partition(ctx, article, outerAfter, limits)
}

func validLimits(ctx context.Context, l Limits) bool {
	_, deadline := ctx.Deadline()
	return deadline && l.MaxBlocks > 0 && l.MaxGroups > 0 && l.MaxInputBytes > 0 && l.MaxPassages > 0 && l.MaxBodyBytes > 0 && l.MaxReferences > 0 && l.MaxContextBytes > 0
}

// Validate counts/strings before allocating per-block/group metadata. Then check
// laminar ranges: groups can nest or be disjoint, but cannot cross one another.
// This validates the in-memory representation, not full HTML grammar/provenance.
func validate(ctx context.Context, a htmlarticle.Article, l Limits) ([]int, error) {
	if len(a.Blocks) == 0 {
		return nil, ErrInvalid
	}
	if len(a.Blocks) > l.MaxBlocks || len(a.Groups) > l.MaxGroups {
		return nil, ErrLimit
	}
	bytes := 0
	admit := func(s string) error {
		if len(s) > l.MaxInputBytes-bytes {
			return ErrLimit
		}
		bytes += len(s)
		if !utf8.ValidString(s) || strings.IndexByte(s, 0) >= 0 {
			return ErrInvalid
		}
		return nil
	}
	if err := admit(a.Processor); err != nil {
		return nil, err
	}
	if err := admit(a.Profile); err != nil {
		return nil, err
	}
	if a.Processor != htmlarticle.Processor {
		return nil, ErrUnsupported
	}
	if !profileID(a.Profile) {
		return nil, ErrInvalid
	}
	for _, b := range a.Blocks {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		for _, s := range []string{string(b.Kind), b.Text, b.Anchor, b.SourceURL} {
			if err := admit(s); err != nil {
				return nil, err
			}
		}
		switch b.Kind {
		case htmlarticle.Heading:
			if b.Level < 1 || b.Level > 6 {
				return nil, ErrInvalid
			}
		case htmlarticle.Prose, htmlarticle.Code, htmlarticle.Image, htmlarticle.Separator:
			if b.Level != 0 {
				return nil, ErrInvalid
			}
		default:
			return nil, ErrUnsupported
		}
		if b.Kind != htmlarticle.Image && b.SourceURL != "" {
			return nil, ErrInvalid
		}
	}
	for _, g := range a.Groups {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := admit(string(g.Kind)); err != nil {
			return nil, err
		}
		if err := admit(g.Label); err != nil {
			return nil, err
		}
		switch g.Kind {
		case htmlarticle.Paragraph, htmlarticle.Notice, htmlarticle.List, htmlarticle.ListItem,
			htmlarticle.DefinitionList, htmlarticle.Term, htmlarticle.Definition, htmlarticle.Table,
			htmlarticle.Row, htmlarticle.Cell, htmlarticle.Figure, htmlarticle.Caption, htmlarticle.Quotation:
		default:
			return nil, ErrUnsupported
		}
		if g.First < 0 || g.After < g.First || g.After > len(a.Blocks) ||
			g.Ordered && g.Kind != htmlarticle.List || g.Header && g.Kind != htmlarticle.Cell ||
			g.Label != "" && g.Kind != htmlarticle.Notice {
			return nil, ErrInvalid
		}
	}
	outerAfter := make([]int, len(a.Blocks))
	var ends []int
	previousFirst := -1
	for _, g := range a.Groups {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if g.First < previousFirst {
			return nil, ErrInvalid
		}
		previousFirst = g.First
		for len(ends) > 0 && g.First >= ends[len(ends)-1] {
			ends = ends[:len(ends)-1]
		}
		if len(ends) > 0 && g.After > ends[len(ends)-1] {
			return nil, ErrInvalid
		}
		if g.First == g.After {
			continue
		}
		if len(ends) == 0 {
			outerAfter[g.First] = g.After
		}
		ends = append(ends, g.After)
	}
	return outerAfter, nil
}

func profileID(s string) bool {
	if len(s) == 0 || len(s) > 128 {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '/' || c == '.') {
			return false
		}
	}
	return true
}
