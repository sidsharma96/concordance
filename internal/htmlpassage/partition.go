package htmlpassage

import (
	"context"

	"github.com/sidsharma96/concordance/internal/htmlarticle"
)

// Each block/group is visited a constant number of times. Heading depth is at
// most six. With K blocks, G groups, T string bytes and R returned references,
// Build's own work is O(K+G+T+R); metadata grows with K+G+P+R for P passages.
// Existing article text remains caller-owned. No text or remote I/O is produced.
func partition(ctx context.Context, a htmlarticle.Article, outerAfter []int, l Limits) (Plan, error) {
	plan := Plan{Processor: Processor}
	var headings, startHeadings [6]int
	headingCount, startCount := 0, 0
	first, bodyBytes, groupCursor, references, contextBytes := 0, 0, 0, 0, 0
	emit := func(after int) error {
		if len(plan.Passages) >= l.MaxPassages {
			return ErrLimit
		}
		groupEnd := groupCursor
		for groupEnd < len(a.Groups) && (a.Groups[groupEnd].First < after || after == len(a.Blocks)) {
			if err := ctx.Err(); err != nil {
				return err
			}
			groupEnd++
		}
		groupCount := groupEnd - groupCursor
		if startCount > l.MaxReferences-references || groupCount > l.MaxReferences-references-startCount {
			return ErrLimit
		}
		references += startCount
		references += groupCount
		charge := func(n int) error {
			if n > l.MaxContextBytes-contextBytes {
				return ErrLimit
			}
			contextBytes += n
			return nil
		}
		for _, index := range startHeadings[:startCount] {
			if err := charge(len(a.Blocks[index].Text)); err != nil {
				return err
			}
		}
		for index := groupCursor; index < groupEnd; index++ {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := charge(len(a.Groups[index].Label)); err != nil {
				return err
			}
		}
		p := Passage{Body: Range{First: first, After: after}}
		p.Headings = append([]int(nil), startHeadings[:startCount]...)
		if groupEnd > groupCursor {
			p.Groups = make([]int, groupEnd-groupCursor)
			for i := range p.Groups {
				p.Groups[i] = groupCursor + i
			}
		}
		plan.Passages = append(plan.Passages, p)
		groupCursor = groupEnd
		first, bodyBytes = after, 0
		return nil
	}
	for i := 0; i < len(a.Blocks); {
		if err := ctx.Err(); err != nil {
			return Plan{}, err
		}
		if i == first {
			startHeadings = headings
			startCount = headingCount
		}
		after := i + 1
		container := outerAfter[i] > i
		if container {
			after = outerAfter[i]
		}
		for index := i; index < after; index++ {
			if err := ctx.Err(); err != nil {
				return Plan{}, err
			}
			b := a.Blocks[index]
			if index > first {
				if bodyBytes == l.MaxBodyBytes {
					return Plan{}, ErrLimit
				}
				bodyBytes++
			}
			if len(b.Text) > l.MaxBodyBytes-bodyBytes {
				return Plan{}, ErrLimit
			}
			bodyBytes += len(b.Text)
			if b.Kind == htmlarticle.Heading {
				// A new peer/ancestor heading inside the body supersedes old
				// context too. Keep only ancestors common to the whole body.
				for startCount > 0 && a.Blocks[startHeadings[startCount-1]].Level >= b.Level {
					startCount--
				}
				for headingCount > 0 && a.Blocks[headings[headingCount-1]].Level >= b.Level {
					headingCount--
				}
				headings[headingCount] = index
				headingCount++
			}
		}
		deferPrefix := !container && (a.Blocks[i].Kind == htmlarticle.Heading || a.Blocks[i].Kind == htmlarticle.Separator)
		if !deferPrefix {
			if err := emit(after); err != nil {
				return Plan{}, err
			}
		}
		i = after
	}
	if first < len(a.Blocks) {
		if err := emit(len(a.Blocks)); err != nil {
			return Plan{}, err
		}
	}
	if err := ctx.Err(); err != nil {
		return Plan{}, err
	}
	return plan, nil
}
