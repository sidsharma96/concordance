package main

import (
	"cmp"
	"context"
	"errors"
	"slices"

	"github.com/sidsharma96/concordance/internal/htmlarticlejson"
	"github.com/sidsharma96/concordance/internal/htmlpassage"
	"github.com/sidsharma96/concordance/internal/termsearch"
)

const anyPolicy = "any-word/single-term-sum-v1"

type anyReport struct {
	Policy    string       `json:"policy"`
	Summaries []summary    `json:"summaries"`
	Cases     []caseReport `json:"cases"`
	Controls  []caseReport `json:"scope_controls"`
}

// This diagnostic composition reuses complete single-term BM25 results. Terms
// contribute in lexical order; absent terms contribute zero. Scratch is O(P+Q),
// where P is the admitted edition's passage count. Each single search also sorts
// its own results. No production query API or postings algorithm is changed.
func rankAny(ctx context.Context, o openedEdition, q termsearch.Query, baseline termsearch.RankedResult) (termsearch.RankedResult, int, int, error) {
	return rankMinimum(ctx, o, q, baseline, 1)
}

func rankMinimum(ctx context.Context, o openedEdition, q termsearch.Query, baseline termsearch.RankedResult, minimum int) (termsearch.RankedResult, int, int, error) {
	fail := func(err error) (termsearch.RankedResult, int, int, error) {
		return termsearch.RankedResult{}, 0, 0, err
	}
	terms := slices.Clone(q.Terms)
	slices.Sort(terms)
	terms = slices.Compact(terms)
	if minimum < 1 || minimum > len(terms) {
		return fail(errors.New("invalid diagnostic minimum term count"))
	}
	scores := make([]float64, len(o.positions))
	counts := make([]int, len(scores))
	total := 0
	for _, term := range terms {
		if err := ctx.Err(); err != nil {
			return fail(err)
		}
		one := q
		one.Terms = []string{term}
		result, err := o.index.Search(ctx, one, queryLimits())
		if err != nil {
			return fail(err)
		}
		if !result.EditionFound || result.RankingVersion != termsearch.BM25Version {
			return fail(errors.New("single-term result contract mismatch"))
		}
		total += len(result.Matches) // At most admitted query terms times admitted passages.
		for _, hit := range result.Matches {
			if err := ctx.Err(); err != nil {
				return fail(err)
			}
			if hit.Passage < 0 || hit.Passage >= len(scores) {
				return fail(errors.New("single-term passage out of range"))
			}
			scores[hit.Passage] += hit.Score
			counts[hit.Passage]++
		}
	}
	ranked := termsearch.RankedResult{EditionFound: baseline.EditionFound, RankingVersion: termsearch.BM25Version, Matches: []termsearch.ScoredPassage{}}
	var intersection []termsearch.ScoredPassage
	for pos, n := range counts {
		if err := ctx.Err(); err != nil {
			return fail(err)
		}
		if n == 0 {
			continue
		}
		hit := termsearch.ScoredPassage{Passage: pos, Score: scores[pos]}
		if n >= minimum {
			ranked.Matches = append(ranked.Matches, hit)
		}
		if n == len(terms) {
			intersection = append(intersection, hit)
		}
	}
	order := func(a, b termsearch.ScoredPassage) int {
		if c := cmp.Compare(b.Score, a.Score); c != 0 {
			return c
		}
		return cmp.Compare(a.Passage, b.Passage)
	}
	slices.SortFunc(ranked.Matches, order)
	slices.SortFunc(intersection, order)
	if !slices.Equal(intersection, baseline.Matches) {
		return fail(errors.New("every-word projection does not reproduce exact baseline ranking"))
	}
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	return ranked, len(terms), total, nil
}

// Quotes use the saved index binding. Context comes from the same exact article's
// canonical public plan and stays separate from verbatim body text. Rebuilding per
// displayed hit is deliberate diagnostic work, not an article cache or free I/O.
func addEvidence(ctx context.Context, o openedEdition, c *caseReport) error {
	for i := range c.Top {
		h := &c.Top[i]
		pos, ok := o.positions[location{h.Path, h.First, h.After}]
		if !ok {
			return errors.New("diagnostic location absent")
		}
		loc, err := o.index.Location(pos)
		if err != nil {
			return err
		}
		limits := buildLimits()
		quote, err := o.index.Quote(ctx, pos, o.bodies[loc.Article], limits.Article, limits.Preparation.Passages.MaxBodyBytes)
		if err != nil {
			return err
		}
		article, err := htmlarticlejson.Decode(ctx, o.bodies[loc.Article], limits.Article)
		if err != nil {
			return err
		}
		plan, err := htmlpassage.Build(ctx, article, limits.Preparation.Passages)
		if err != nil {
			return err
		}
		found := 0
		var headings, labels []string
		for _, p := range plan.Passages {
			if p.Body != loc.Body {
				continue
			}
			found++
			for _, hi := range p.Headings {
				headings = append(headings, article.Blocks[hi].Text)
			}
			for _, gi := range p.Groups {
				if label := article.Groups[gi].Label; label != "" {
					labels = append(labels, label)
				}
			}
		}
		if found != 1 {
			return errors.New("diagnostic context does not bind unique saved range")
		}
		h.ArticleSHA256 = o.docs[loc.Article].ArticleSHA256
		h.Quote, h.Headings, h.Labels = quote, headings, labels
	}
	return ctx.Err()
}
