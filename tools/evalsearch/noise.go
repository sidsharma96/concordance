package main

import (
	"context"
	"errors"
	"slices"

	"github.com/sidsharma96/concordance/internal/termsearch"
	"github.com/sidsharma96/concordance/internal/textterms"
)

// Fixed before the experiment. This is a query-only hypothesis, not a product
// tokenizer or a claim that these words never carry meaning. Do not tune in place.
const (
	questionPolicy = "question-words/all-retained-v1"
	halfPolicy     = "minimum-half/raw-terms-v1"
	questionWords  = "a an are can do does how i is it my of on the to what when where which why"
)

type noiseCase struct {
	caseReport
	RetainedTerms []string `json:"retained_terms"`
	RemovedTerms  []string `json:"removed_terms"`
	MinimumMatch  int      `json:"minimum_match"`
	QueryStatus   string   `json:"query_status"`
}
type noiseReport struct {
	Policy       string      `json:"policy"`
	RemovedWords []string    `json:"removed_words,omitempty"`
	Summaries    []summary   `json:"summaries"`
	Cases        []noiseCase `json:"cases"`
	Controls     []noiseCase `json:"scope_controls"`
}

// Return independent slice storage in sorted distinct order. Strings borrow the
// immutable prepared text; no caller slice is sorted or rewritten. At most 64
// admitted occurrences and a constant twenty-word membership test are processed.
func removeQuestionWords(input []string) (retained, removed []string) {
	terms := slices.Clone(input)
	slices.Sort(terms)
	terms = slices.Compact(terms)
	retained, removed = []string{}, []string{}
	for _, term := range terms {
		switch term {
		case "a", "an", "are", "can", "do", "does", "how", "i", "is", "it", "my", "of", "on", "the", "to", "what", "when", "where", "which", "why":
			removed = append(removed, term)
		default:
			retained = append(retained, term)
		}
	}
	return retained, removed
}

// Synchronous diagnostic work only. Repeats preparation and original Search so
// this pair validates its own inputs. Scores/indices/corpus statistics stay owned
// by existing components; returned reports own their slices and evidence strings.
func searchNoisePair(ctx context.Context, o openedEdition, id, form, raw string, refs []reference) ([2]noiseCase, error) {
	fail := func(err error) ([2]noiseCase, error) { return [2]noiseCase{}, err }
	prepared, err := textterms.Prepare(ctx, raw, textterms.Limits{MaxInputBytes: 4096, MaxTerms: 64, MaxTermBytes: 4096, MaxOutputBytes: 4096})
	if err != nil {
		return fail(err)
	}
	if len(prepared.Terms) == 0 {
		return fail(errors.New("query has no technical terms"))
	}
	q := termsearch.Query{Edition: o.index.ID(), TermsVersion: prepared.Version, Terms: prepared.Terms}
	baseline, err := o.index.Search(ctx, q, queryLimits())
	if err != nil {
		return fail(err)
	}
	retained, removed := removeQuestionWords(q.Terms)
	filtered := termsearch.RankedResult{EditionFound: true, RankingVersion: termsearch.BM25Version}
	status := "ready"
	if len(retained) == 0 {
		status = "all_terms_removed"
	} else {
		only := q
		only.Terms = retained
		filtered, err = o.index.Search(ctx, only, queryLimits())
		if err != nil {
			return fail(err)
		}
	}
	c, err := describeCase(ctx, o, id, form, refs, filtered)
	if err != nil {
		return fail(err)
	}
	out := [2]noiseCase{{caseReport: c, RetainedTerms: retained, RemovedTerms: removed, MinimumMatch: len(retained), QueryStatus: status}}
	terms := slices.Clone(q.Terms)
	slices.Sort(terms)
	terms = slices.Compact(terms)
	minimum := (len(terms) + 1) / 2 // Nonempty original query; round half upward.
	ranked, distinct, hits, err := rankMinimum(ctx, o, q, baseline, minimum)
	if err != nil {
		return fail(err)
	}
	c, err = describeCase(ctx, o, id, form, refs, ranked)
	if err != nil {
		return fail(err)
	}
	c.DistinctTerms, c.SingleTermHits = distinct, hits
	out[1] = noiseCase{caseReport: c, RetainedTerms: terms, RemovedTerms: []string{}, MinimumMatch: minimum, QueryStatus: "ready"}
	for i := range out {
		if err := addEvidence(ctx, o, &out[i].caseReport); err != nil {
			return fail(err)
		}
	}
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	return out, nil
}
