package termsearch

import (
	"cmp"
	"context"
	"slices"
)

type indexSelection struct {
	edition   *indexedEdition
	lists     []*postingList // distinct terms in lexical order, not intersection order
	positions []int
}

// MatchAll intersects sorted posting lists, returning the same complete, ascending
// passage positions as the scan. Limits admit the ORIGINAL all-edition input plus
// query, using cached counts/bytes rather than rescanning input. MaxMatches applies
// only to final output; temporary candidates can reach the shortest posting-list
// length, bounded by admitted passages. Queries borrow aliases until return and
// return owned output with no retained query input. Errors return a zero Result.
func (x *Index) MatchAll(ctx context.Context, q Query, l Limits) (Result, error) {
	selected, err := x.selectMatches(ctx, q, l)
	if err != nil {
		return Result{}, err
	}
	positions := selected.positions
	// Filtering may leave a tiny result backed by a large candidate allocation.
	// Copy only when pruned; Clip alone would still retain that allocation.
	if len(positions) > 0 && cap(positions) > len(positions) {
		positions = slices.Clone(positions)
	}
	return Result{EditionFound: selected.edition != nil, Passages: positions}, nil
}

func (x *Index) queryLists(ctx context.Context, q Query, l Limits) (indexSelection, error) {
	zero := indexSelection{}
	if err := indexContext(ctx); err != nil {
		return zero, err
	}
	if x == nil || x.data == nil || !validLimits(l) {
		return zero, ErrInvalid
	}
	d := x.data
	if d.input.editions > l.MaxEditions || d.input.passages > l.MaxPassages || d.input.terms > l.MaxTerms || len(q.Terms) > l.MaxQueryTerms {
		return zero, ErrLimit
	}
	if len(q.Terms) == 0 {
		return zero, ErrInvalid
	}
	remaining := l.MaxInputBytes
	if d.input.bytes > remaining {
		return zero, ErrLimit
	}
	remaining -= d.input.bytes
	for _, s := range []string{q.TermsVersion, q.Edition} {
		if len(s) > remaining {
			return zero, ErrLimit
		}
		remaining -= len(s)
	}
	for _, s := range q.Terms {
		if err := ctx.Err(); err != nil {
			return zero, err
		}
		if len(s) > remaining {
			return zero, ErrLimit
		}
		remaining -= len(s)
	}
	if !validString(q.TermsVersion) || !validString(q.Edition) {
		return zero, ErrInvalid
	}
	if q.TermsVersion != d.version {
		return zero, ErrIncompatible
	}
	distinct := make(map[string]struct{}, len(q.Terms))
	for _, s := range q.Terms {
		if err := ctx.Err(); err != nil {
			return zero, err
		}
		if !validString(s) {
			return zero, ErrInvalid
		}
		distinct[s] = struct{}{}
	}
	terms := make([]string, 0, len(distinct))
	for term := range distinct {
		if err := ctx.Err(); err != nil {
			return zero, err
		}
		terms = append(terms, term)
	}
	slices.Sort(terms)
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	e := d.editions[q.Edition]
	if e == nil {
		return zero, ctx.Err()
	}
	selected := indexSelection{edition: e, lists: make([]*postingList, 0, len(terms))}
	for _, term := range terms {
		if err := ctx.Err(); err != nil {
			return zero, err
		}
		list := e.terms[term]
		selected.lists = append(selected.lists, list)
	}
	return selected, ctx.Err()
}

func (x *Index) selectMatches(ctx context.Context, q Query, l Limits) (indexSelection, error) {
	zero := indexSelection{}
	selected, err := x.queryLists(ctx, q, l)
	if err != nil || selected.edition == nil {
		return selected, err
	}
	for _, list := range selected.lists {
		if err := ctx.Err(); err != nil {
			return zero, err
		}
		if list == nil {
			return indexSelection{edition: selected.edition}, ctx.Err()
		}
	}
	order := make([]int, len(selected.lists))
	for i := range order {
		order[i] = i
	}
	slices.SortFunc(order, func(a, b int) int {
		if c := cmp.Compare(len(selected.lists[a].rows), len(selected.lists[b].rows)); c != 0 {
			return c
		}
		return cmp.Compare(a, b)
	})
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	shortest := selected.lists[order[0]].rows
	candidates := make([]int, 0, len(shortest))
	for _, row := range shortest {
		if err := ctx.Err(); err != nil {
			return zero, err
		}
		candidates = append(candidates, row.passage)
	}
	for _, index := range order[1:] {
		rows := selected.lists[index].rows
		// Reuse only local scratch. The output prefix never overtakes unread input.
		out := candidates[:0]
		i, j := 0, 0
		for i < len(candidates) && j < len(rows) {
			if err := ctx.Err(); err != nil {
				return zero, err
			}
			a, b := candidates[i], rows[j].passage
			switch {
			case a < b:
				i++
			case a > b:
				j++
			default:
				out = append(out, a)
				i++
				j++
			}
		}
		candidates = out
		if len(candidates) == 0 {
			break
		}
	}
	if len(candidates) > l.MaxMatches {
		return zero, ErrLimit
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	if len(candidates) > 0 {
		selected.positions = candidates
	}
	return selected, nil
}
