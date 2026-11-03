package termsearch

import (
	"cmp"
	"context"
	"math"
	"slices"
)

// AnyBM25Version identifies union eligibility with the existing combined-field
// BM25 formula, distinct lexical-term sum order and input-position ties.
const AnyBM25Version = "any-word/single-term-sum-v1"

// RankBM25Any returns every passage containing at least one query term. Missing
// terms contribute nothing; duplicate terms contribute once. RankBM25 remains
// match-all. The same original-input/query admission, exact-edition statistics,
// ownership and deadline rules apply. Excess complete union matches are errors,
// never permission to truncate to a candidate window. Errors return zero results.
//
// For P selected passages, Q distinct query terms, L queried posting rows and M
// union matches, work is O(Q log Q + L + P + M log M), plus validation and string
// lookup/comparison costs. Scratch is O(P + Q + M). No I/O, query retention or
// goroutines; immutable index state is shared, query scratch/output is owned.
func (x *Index) RankBM25Any(ctx context.Context, q Query, l Limits) (RankedResult, error) {
	selected, err := x.queryLists(ctx, q, l)
	if err != nil {
		return RankedResult{}, err
	}
	result := RankedResult{EditionFound: selected.edition != nil, RankingVersion: AnyBM25Version}
	e := selected.edition
	if e == nil || len(e.lengths) == 0 {
		return result, nil
	}
	scores := make([]float64, len(e.lengths))
	average := float64(e.totalLength) / float64(len(e.lengths))
	matched := 0
	for _, list := range selected.lists { // queryLists preserves lexical term order.
		if err := ctx.Err(); err != nil {
			return RankedResult{}, err
		}
		if list == nil {
			continue
		}
		df := len(list.rows)
		idf := math.Log1p((float64(len(e.lengths)-df) + 0.5) / (float64(df) + 0.5))
		for _, row := range list.rows {
			if err := ctx.Err(); err != nil {
				return RankedResult{}, err
			}
			if scores[row.passage] == 0 { // Every contribution is strictly positive.
				if matched == l.MaxMatches {
					return RankedResult{}, ErrLimit
				}
				matched++
			}
			adjustment := bm25K1 * (1 - bm25B + bm25B*float64(e.lengths[row.passage])/average)
			tf := float64(row.frequency())
			scores[row.passage] += idf * tf * (bm25K1 + 1) / (tf + adjustment)
		}
	}
	result.Matches = make([]ScoredPassage, 0, matched)
	for position, score := range scores {
		if err := ctx.Err(); err != nil {
			return RankedResult{}, err
		}
		if score > 0 {
			result.Matches = append(result.Matches, ScoredPassage{Passage: position, Score: score})
		}
	}
	slices.SortFunc(result.Matches, func(a, b ScoredPassage) int {
		if c := cmp.Compare(b.Score, a.Score); c != 0 {
			return c
		}
		return cmp.Compare(a.Passage, b.Passage)
	})
	if err := ctx.Err(); err != nil {
		return RankedResult{}, err
	}
	return result, nil
}
