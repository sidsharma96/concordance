package termsearch

import (
	"cmp"
	"context"
	"math"
	"slices"
)

// BM25Version identifies the combined-field formula, fixed parameters, edition
// statistics (including empty passages), distinct-term sum order and tie rule.
// It is separate from the caller's term-preparation version and generation.
const BM25Version = "bm25/combined-v1"

const (
	bm25K1 = 1.2
	bm25B  = 0.75
)

// ScoredPassage is a temporary position in the selected edition and its score.
// Score is finite and positive; it is not a probability or comparable confidence
// across queries or editions. Passage is not a persistent identity or citation.
type ScoredPassage struct {
	Passage int
	Score   float64
}

// RankedResult owns Matches, ordered by decreasing score, then ascending input
// position for exact ties. EditionFound distinguishes absent and empty editions.
// RankingVersion is BM25Version on every successful return, including no matches.
type RankedResult struct {
	EditionFound   bool
	RankingVersion string
	Matches        []ScoredPassage
}

// RankBM25 ranks the complete MatchAll result using combined body/heading/label
// occurrence counts. IDF and average length use all passages in the exact selected
// edition, including empty passages, not just matches. Repeated query terms count
// once. Distinct terms are summed in lexical order. Parameters are k1=1.2, b=0.75:
//
//	idf = ln(1 + (N - df + .5) / (df + .5))
//	contribution = idf * tf * (k1 + 1) /
//	    (tf + k1 * (1 - b + b * length / averageLength))
//
// All MatchAll admission, validation, ownership and deadline rules apply. Limits
// count all supplied editions, while scoring statistics use only the selected one.
// MaxMatches rejects excess instead of truncating. Any failure returns a zero
// RankedResult. Input must remain unchanged until return; output retains no input.
// No I/O, goroutines or shared state. Cancellation is cooperative; bounded sorts
// finish before cancellation is returned, rather than being interrupted mid-sort.
//
// Expected additional work after matching is O(T + P*Q + M*Q + Q log Q + M log M)
// for selected occurrences T, passages P, distinct query terms Q and matches M,
// under ordinary map assumptions. Additional space is O(Q + M), not P*Q.
func RankBM25(ctx context.Context, corpus Corpus, query Query, limits Limits) (RankedResult, error) {
	candidates, err := MatchAll(ctx, corpus, query, limits)
	if err != nil {
		return RankedResult{}, err
	}
	result := RankedResult{EditionFound: candidates.EditionFound, RankingVersion: BM25Version}
	if len(candidates.Passages) == 0 {
		if err := ctx.Err(); err != nil {
			return RankedResult{}, err
		}
		return result, nil
	}

	positions := make(map[string]int)
	for _, term := range query.Terms {
		if err := ctx.Err(); err != nil {
			return RankedResult{}, err
		}
		positions[term] = 0
	}
	terms := make([]string, 0, len(positions))
	for term := range positions {
		if err := ctx.Err(); err != nil {
			return RankedResult{}, err
		}
		terms = append(terms, term)
	}
	slices.Sort(terms)
	for i, term := range terms {
		if err := ctx.Err(); err != nil {
			return RankedResult{}, err
		}
		positions[term] = i
	}
	var passages []Fields
	for _, edition := range corpus.Editions {
		if err := ctx.Err(); err != nil {
			return RankedResult{}, err
		}
		if edition.ID == query.Edition {
			passages = edition.Passages
			break
		}
	}

	counts := make([]int, len(terms))
	df := make([]int, len(terms))
	totalLength := 0
	for _, passage := range passages {
		length, err := countQueryTerms(ctx, passage, positions, counts)
		if err != nil {
			return RankedResult{}, err
		}
		// MatchAll admitted the sum of every occurrence before these additions.
		totalLength += length
		for i, count := range counts {
			if err := ctx.Err(); err != nil {
				return RankedResult{}, err
			}
			if count > 0 {
				df[i]++
			}
		}
	}
	average := float64(totalLength) / float64(len(passages))
	idf := make([]float64, len(terms))
	for i, frequency := range df {
		if err := ctx.Err(); err != nil {
			return RankedResult{}, err
		}
		idf[i] = math.Log1p((float64(len(passages)-frequency) + 0.5) / (float64(frequency) + 0.5))
	}
	result.Matches = make([]ScoredPassage, 0, len(candidates.Passages))
	for _, index := range candidates.Passages {
		length, err := countQueryTerms(ctx, passages[index], positions, counts)
		if err != nil {
			return RankedResult{}, err
		}
		adjustment := bm25K1 * (1 - bm25B + bm25B*float64(length)/average)
		score := 0.0
		for i, count := range counts {
			if err := ctx.Err(); err != nil {
				return RankedResult{}, err
			}
			tf := float64(count)
			score += idf[i] * tf * (bm25K1 + 1) / (tf + adjustment)
		}
		result.Matches = append(result.Matches, ScoredPassage{Passage: index, Score: score})
	}
	slices.SortFunc(result.Matches, func(a, b ScoredPassage) int {
		if order := cmp.Compare(b.Score, a.Score); order != 0 {
			return order
		}
		return cmp.Compare(a.Passage, b.Passage)
	})
	if err := ctx.Err(); err != nil {
		return RankedResult{}, err
	}
	return result, nil
}

// counts is caller-owned scratch, reset for each passage. Count all occurrences
// for length, but retain frequencies only for query terms. No field weights.
func countQueryTerms(ctx context.Context, passage Fields, positions map[string]int, counts []int) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	clear(counts)
	length := 0
	for _, field := range [3][]string{passage.Body, passage.Headings, passage.Labels} {
		for _, term := range field {
			if err := ctx.Err(); err != nil {
				return 0, err
			}
			length++
			if i, ok := positions[term]; ok {
				counts[i]++
			}
		}
	}
	return length, nil
}
