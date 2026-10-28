package termsearch

import (
	"cmp"
	"context"
	"math"
	"slices"
)

// RankBM25 uses posting frequencies and cached edition lengths, with the exact
// scan formula, lexical distinct-term sum order and stable input-position ties.
// Statistics include every passage in the selected edition, including empty ones.
// MatchAll's limits/ownership apply. No partial output or top-k truncation.
//
// For Q distinct query words, L total queried posting lengths and M final matches,
// query work includes O(L + M*Q + Q log Q + M log M), plus query byte validation and
// string lookup/comparison costs. Scratch is O(Q + shortest-list length + M).
// Two-pointer scans can visit long common-word lists even with few final matches.
// Bounded library sorts finish before cancellation is returned. No I/O or goroutines.
func (x *Index) RankBM25(ctx context.Context, q Query, l Limits) (RankedResult, error) {
	selected, err := x.selectMatches(ctx, q, l)
	if err != nil {
		return RankedResult{}, err
	}
	result := RankedResult{EditionFound: selected.edition != nil, RankingVersion: BM25Version}
	if len(selected.positions) == 0 {
		return result, nil
	}
	e := selected.edition
	average := float64(e.totalLength) / float64(len(e.lengths))
	idf := make([]float64, len(selected.lists))
	cursors := make([]int, len(selected.lists))
	for i, list := range selected.lists {
		if err := ctx.Err(); err != nil {
			return RankedResult{}, err
		}
		df := len(list.rows)
		idf[i] = math.Log1p((float64(len(e.lengths)-df) + 0.5) / (float64(df) + 0.5))
	}
	result.Matches = make([]ScoredPassage, 0, len(selected.positions))
	for _, position := range selected.positions {
		if err := ctx.Err(); err != nil {
			return RankedResult{}, err
		}
		adjustment := bm25K1 * (1 - bm25B + bm25B*float64(e.lengths[position])/average)
		score := 0.0
		for i, list := range selected.lists {
			if err := ctx.Err(); err != nil {
				return RankedResult{}, err
			}
			for list.rows[cursors[i]].passage < position {
				if err := ctx.Err(); err != nil {
					return RankedResult{}, err
				}
				cursors[i]++
			}
			// The complete intersection guarantees this row exists and has this position.
			tf := float64(list.rows[cursors[i]].frequency())
			score += idf[i] * tf * (bm25K1 + 1) / (tf + adjustment)
		}
		result.Matches = append(result.Matches, ScoredPassage{Passage: position, Score: score})
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
