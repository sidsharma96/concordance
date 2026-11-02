package main

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/sidsharma96/concordance/internal/capturefiles"
	"github.com/sidsharma96/concordance/internal/htmlarticlejson"
	"github.com/sidsharma96/concordance/internal/htmlterms"
	"github.com/sidsharma96/concordance/internal/termsearch"
	"github.com/sidsharma96/concordance/internal/textterms"
)

func TestQuestionFilterOwnsSlicesAndKeepsQualifiers(t *testing.T) {
	input := []string{"why", "retry", "not", "before", "after", "without", "no", "nor", "is.ready", "initContainer", "retry", "my", "why"}
	original := slices.Clone(input)
	kept, removed := removeQuestionWords(input)
	if !slices.Equal(input, original) || !slices.Equal(removed, []string{"my", "why"}) || !slices.Equal(kept, []string{"after", "before", "initContainer", "is.ready", "no", "nor", "not", "retry", "without"}) {
		t.Fatal("filter changed input, qualifiers or exact technical terms", kept, removed)
	}
	kept[0], removed[0] = "changed", "changed"
	if !slices.Equal(input, original) {
		t.Fatal("result slices alias input")
	}
	// Keep declared metadata identical to the actual fixed membership predicate.
	kept, removed = removeQuestionWords(strings.Fields(questionWords))
	if len(kept) != 0 || !slices.Equal(removed, strings.Fields(questionWords)) || len(removed) != 20 {
		t.Fatal("fixed list differs from filter", kept, removed)
	}
}

func TestNoiseComparisonGuardsStatusesAndFailures(t *testing.T) {
	root, path, s := fixture(t)
	s.Intents[0].Natural = "Why is my retry safely"
	writeSet(t, path, s)
	args := []string{"-capture", root, "-set", path, "-compare-noise"}
	for _, split := range []string{"reserved", "all"} {
		var out bytes.Buffer
		if err := run(t.Context(), append(slices.Clone(args), "-split", split), &out); err == nil || out.Len() != 0 {
			t.Fatal("noise comparison admitted forbidden split", split)
		}
	}
	var out bytes.Buffer
	if err := run(t.Context(), args, &out); err != nil {
		t.Fatal(err)
	}
	var r report
	if err := json.Unmarshal(out.Bytes(), &r); err != nil {
		t.Fatal(err)
	}
	if r.AnyWord == nil || len(r.Noise) != 2 || r.Noise[0].Policy != questionPolicy || r.Noise[1].Policy != halfPolicy || len(r.Noise[0].Cases) != 2 || len(r.Noise[1].Controls) != 1 || r.References != 2 {
		t.Fatal("missing baseline/methods/cases")
	}
	filtered, half := r.Noise[0].Cases[1], r.Noise[1].Cases[1]
	if filtered.Matches != 2 || filtered.MinimumMatch != 2 || filtered.QueryStatus != "ready" || !slices.Equal(filtered.RetainedTerms, []string{"retry", "safely"}) || !slices.Equal(filtered.RemovedTerms, []string{"is", "my", "why"}) || half.Matches != 0 || half.MinimumMatch != 3 || half.DistinctTerms != 5 {
		t.Fatal("wrong eligibility or denominator", filtered, half)
	}
	if filtered.Top[0].Judgment != "unjudged" || filtered.Top[1].Judgment != "known_useful" || filtered.Top[0].Quote != "Pods\nretry safely" || !digest(filtered.Top[0].ArticleSHA256) || len(filtered.Top[0].Headings) != 0 {
		t.Fatal("lost exact evidence or logical identity", filtered.Top)
	}
	for _, n := range r.Noise {
		for _, c := range n.Cases {
			if c.ID == "reserved-question" {
				t.Fatal("reserved question executed")
			}
		}
	}
	if err := run(t.Context(), args, shortWriter{}); !errors.Is(err, io.ErrShortWrite) {
		t.Fatal("short output hidden", err)
	}
	s.Intents[0].Natural = "why is my"
	writeSet(t, path, s)
	out.Reset()
	if err := run(t.Context(), args, &out); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(out.Bytes(), &r); err != nil {
		t.Fatal(err)
	}
	filtered = r.Noise[0].Cases[1]
	if filtered.QueryStatus != "all_terms_removed" || filtered.MinimumMatch != 0 || len(filtered.RetainedTerms) != 0 || filtered.Matches != 0 || filtered.FirstKnownRank != 0 || len(filtered.Top) != 0 {
		t.Fatal("all-removed query had fallback or was hidden", filtered)
	}
	for _, fault := range []string{"invalid-query", "reserved-binding", "canceled"} {
		t.Run(fault, func(t *testing.T) {
			ctx := t.Context()
			bad := s
			bad.Intents = slices.Clone(s.Intents)
			switch fault {
			case "invalid-query":
				bad.Intents[0].Natural = "!!!"
			case "reserved-binding":
				bad.Intents[1].Answers = []answer{{Edition: "one", References: []reference{{Path: "docs/b.md", First: 0, After: 2, QuoteSHA256: strings.Repeat("0", 64)}}}}
			case "canceled":
				dead, cancel := context.WithCancel(ctx)
				cancel()
				ctx = dead
			}
			writeSet(t, path, bad)
			out.Reset()
			if err := run(ctx, args, &out); err == nil || out.Len() != 0 {
				t.Fatal("fault became a relevance miss/partial report", fault, err)
			}
		})
	}
}

func TestMinimumEligibilityAgainstIndependentScan(t *testing.T) {
	root, _, set := fixture(t)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	inputs, err := capturefiles.ReadDirectory(ctx, root, captureLimits())
	if err != nil {
		t.Fatal(err)
	}
	o, err := openEdition(ctx, inputs[0], set)
	if err != nil {
		t.Fatal(err)
	}
	corpus := termsearch.Corpus{TermsVersion: textterms.Version, Editions: []termsearch.Edition{{ID: o.index.ID()}}}
	for _, body := range o.bodies {
		a, err := htmlarticlejson.Decode(ctx, body, buildLimits().Article)
		if err != nil {
			t.Fatal(err)
		}
		p, err := htmlterms.Build(ctx, a, buildLimits().Preparation)
		if err != nil {
			t.Fatal(err)
		}
		corpus.Editions[0].Passages = append(corpus.Editions[0].Passages, p.Fields...)
	}
	for _, raw := range []string{"retry", "missing", "retry safely", "pods retry safely", "missing retry safely", "missing absent retry safely", "safely retry retry missing", "pods unrelated words"} {
		p, err := textterms.Prepare(ctx, raw, buildLimits().Preparation.Field)
		if err != nil {
			t.Fatal(err)
		}
		q := termsearch.Query{Edition: o.index.ID(), TermsVersion: p.Version, Terms: p.Terms}
		base, err := o.index.Search(ctx, q, queryLimits())
		if err != nil {
			t.Fatal(err)
		}
		terms := slices.Clone(q.Terms)
		slices.Sort(terms)
		terms = slices.Compact(terms)
		// Independent scan scores, plus flat field membership per distinct word.
		scores := make([]float64, len(o.positions))
		for _, term := range terms {
			one := q
			one.Terms = []string{term}
			got, err := termsearch.RankBM25(ctx, corpus, one, queryLimits())
			if err != nil {
				t.Fatal(err)
			}
			for _, hit := range got.Matches {
				scores[hit.Passage] += hit.Score
			}
		}
		for minimum := 1; minimum <= len(terms); minimum++ {
			var want []termsearch.ScoredPassage
			for pos, field := range corpus.Editions[0].Passages {
				count := 0
				for _, term := range terms {
					if slices.Contains(field.Body, term) || slices.Contains(field.Headings, term) || slices.Contains(field.Labels, term) {
						count++
					}
				}
				if count >= minimum {
					want = append(want, termsearch.ScoredPassage{Passage: pos, Score: scores[pos]})
				}
			}
			slices.SortFunc(want, func(a, b termsearch.ScoredPassage) int {
				if c := cmp.Compare(b.Score, a.Score); c != 0 {
					return c
				}
				return cmp.Compare(a.Passage, b.Passage)
			})
			got, distinct, _, err := rankMinimum(ctx, o, q, base, minimum)
			if err != nil || distinct != len(terms) || !slices.Equal(got.Matches, want) {
				t.Fatalf("query %q minimum %d: got%+v want%+v error%v", raw, minimum, got, want, err)
			}
		}
		for _, minimum := range []int{0, -1, len(terms) + 1} {
			got, _, _, err := rankMinimum(ctx, o, q, base, minimum)
			if err == nil || len(got.Matches) != 0 {
				t.Fatal("invalid minimum accepted", minimum)
			}
		}
	}
}
