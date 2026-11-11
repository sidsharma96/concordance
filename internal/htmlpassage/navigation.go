package htmlpassage

import (
	"context"
	"errors"

	"github.com/sidsharma96/concordance/internal/htmlarticle"
)

// Scope selects authored context, not a new matching or passage grouping policy.
type Scope string

const (
	Exact    Scope = "exact"
	Section  Scope = "section"
	Parent   Scope = "parent"
	Previous Scope = "previous"
	Next     Scope = "next"
)

var ErrNoContext = errors.New("no surrounding HTML context in that direction")

// Navigation owns ranges into the unchanged article. Zero adjacent/parent ranges
// mean unavailable. Body always starts and ends on complete canonical passages.
type Navigation struct {
	Body, Previous, Next, Parent Range
	Headings                     []int
}

// Navigate validates/builds the canonical plan before accepting anchor boundaries.
// It borrows article through return and copies no text. Outer heading sections end
// at their next peer/ancestor heading; headings inside authored containers cannot
// split sections. Boundaries expand outward to whole passages. Parent is strictly
// larger; the whole document is the final parent. No automatic paging/truncation.
// Beyond Build, work and scratch are O(blocks + groups + passages); heading depth
// is at most six. Deadline checks are cooperative; no goroutines or I/O occur.
func Navigate(ctx context.Context, a htmlarticle.Article, anchor Range, scope Scope, limits Limits) (Navigation, error) {
	if ctx == nil {
		return Navigation{}, ErrInvalid
	}
	if scope != Exact && scope != Section && scope != Parent && scope != Previous && scope != Next {
		return Navigation{}, ErrInvalid
	}
	plan, err := Build(ctx, a, limits)
	if err != nil {
		return Navigation{}, err
	}
	first, after := -1, -1
	for i, p := range plan.Passages {
		if err := ctx.Err(); err != nil {
			return Navigation{}, err
		}
		if p.Body.First == anchor.First {
			first = i
		}
		if p.Body.After == anchor.After {
			after = i + 1
		}
	}
	if first < 0 || after <= first {
		return Navigation{}, ErrInvalid
	}
	// Map block boundaries to whole passages once. Snapping each section is O(1).
	starts, ends := make([]int, len(a.Blocks)), make([]int, len(a.Blocks))
	for _, p := range plan.Passages {
		for i := p.Body.First; i < p.Body.After; i++ {
			starts[i], ends[i] = p.Body.First, p.Body.After
		}
	}
	covered := make([]int, len(a.Blocks)+1)
	for _, g := range a.Groups {
		if err := ctx.Err(); err != nil {
			return Navigation{}, err
		}
		if g.Kind != htmlarticle.Paragraph && g.First < g.After {
			covered[g.First]++
			covered[g.After]--
		}
	}
	sections := []Range{{0, len(a.Blocks)}}
	stack := make([]int, 0, 6)
	headingEnds := make([]int, len(a.Blocks))
	closeSection := func(end int) {
		h := stack[len(stack)-1]
		headingEnds[h] = end
		stack = stack[:len(stack)-1]
		sections = append(sections, Range{starts[h], ends[end-1]})
	}
	depth := 0
	for i, b := range a.Blocks {
		if err := ctx.Err(); err != nil {
			return Navigation{}, err
		}
		depth += covered[i]
		if b.Kind != htmlarticle.Heading || depth != 0 {
			continue
		}
		for len(stack) > 0 && a.Blocks[stack[len(stack)-1]].Level >= b.Level {
			closeSection(i)
		}
		stack = append(stack, i)
	}
	for len(stack) > 0 {
		closeSection(len(a.Blocks))
	}
	enclosing := func(body Range, strict bool) Range {
		var best Range
		for _, s := range sections {
			if s.First <= body.First && s.After >= body.After && (!strict || s != body) && (best.After == 0 || s.After-s.First < best.After-best.First) {
				best = s
			}
		}
		return best
	}
	body := anchor
	switch scope {
	case Section:
		body = enclosing(anchor, false)
	case Parent:
		body = enclosing(anchor, true)
	case Previous:
		if first > 0 {
			body = plan.Passages[first-1].Body
		} else {
			body = Range{}
		}
	case Next:
		if after < len(plan.Passages) {
			body = plan.Passages[after].Body
		} else {
			body = Range{}
		}
	}
	if body.After == 0 {
		return Navigation{}, ErrNoContext
	}
	out := Navigation{Body: body, Parent: enclosing(body, true)}
	for i, end := range headingEnds {
		if err := ctx.Err(); err != nil {
			return Navigation{}, err
		}
		if i < body.First && end >= body.After {
			out.Headings = append(out.Headings, i)
		}
	}
	for _, p := range plan.Passages {
		if err := ctx.Err(); err != nil {
			return Navigation{}, err
		}
		if p.Body.After == body.First {
			out.Previous = p.Body
		}
		if p.Body.First == body.After {
			out.Next = p.Body
		}
	}
	if err := ctx.Err(); err != nil {
		return Navigation{}, err
	}
	return out, nil
}
