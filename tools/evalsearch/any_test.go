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
	"github.com/sidsharma96/concordance/internal/htmlarticle"
	"github.com/sidsharma96/concordance/internal/htmlarticlejson"
	"github.com/sidsharma96/concordance/internal/htmledition"
	"github.com/sidsharma96/concordance/internal/htmlterms"
	"github.com/sidsharma96/concordance/internal/termsearch"
	"github.com/sidsharma96/concordance/internal/textterms"
)

func TestComparisonDevelopmentGuardAndEvidence(t *testing.T) {
	root, path, s := fixture(t)
	s.Intents[0].Natural = "unrelated absentword"
	writeSet(t, path, s)
	args := []string{"-capture", root, "-set", path, "-compare-any"}
	for _, split := range []string{"reserved", "all"} {
		var out bytes.Buffer
		if err := run(t.Context(), append(slices.Clone(args), "-split", split), &out); err == nil || out.Len() != 0 {
			t.Fatal("comparison executed a forbidden split", split)
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
	if r.AnyWord == nil || r.AnyWord.Policy != anyPolicy || len(r.AnyWord.Cases) != 2 || len(r.AnyWord.Controls) != 1 || r.References != 2 {
		t.Fatal("incomplete comparison")
	}
	if r.Cases[1].Matches != 0 || r.AnyWord.Cases[1].Matches != 2 || r.AnyWord.Cases[1].SingleTermHits != 2 {
		t.Fatal("missing query term excluded all candidates")
	}
	for _, c := range r.AnyWord.Cases {
		if c.ID == "reserved-question" {
			t.Fatal("reserved query executed")
		}
		for _, h := range c.Top {
			if !digest(h.ArticleSHA256) || h.Quote == "" {
				t.Fatal("missing evidence")
			}
		}
	}
	h := r.AnyWord.Cases[1].Top[0]
	if h.Quote != "unrelated words" || !slices.Equal(h.Headings, []string{"Pods"}) || h.Judgment != "unjudged" {
		t.Fatalf("context added to body or invented relevance: %+v", h)
	}
	if r.AnyWord.Cases[0].Top[0].Judgment != "unjudged" || r.AnyWord.Cases[0].Top[1].Judgment != "known_useful" {
		t.Fatal("shared article bytes collapsed document identity")
	}
	if err := run(t.Context(), args, shortWriter{}); !errors.Is(err, io.ErrShortWrite) {
		t.Fatal("comparison short write was hidden", err)
	}
	// Every reference, including reserved, must still pass full preflight.
	s.Intents[1].Answers[0].References[0].QuoteSHA256 = strings.Repeat("0", 64)
	writeSet(t, path, s)
	out.Reset()
	if err := run(t.Context(), args, &out); err == nil || out.Len() != 0 {
		t.Fatal("bad reserved binding became a reported relevance miss")
	}
}

func TestAnyRankingAgainstScanAndExactIntersection(t *testing.T) {
	root, _, s := fixture(t)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	inputs, err := capturefiles.ReadDirectory(ctx, root, captureLimits())
	if err != nil {
		t.Fatal(err)
	}
	o, err := openEdition(ctx, inputs[0], s)
	if err != nil {
		t.Fatal(err)
	}
	corpus := termsearch.Corpus{TermsVersion: textterms.Version, Editions: []termsearch.Edition{{ID: o.index.ID()}}}
	for _, b := range o.bodies {
		a, err := htmlarticlejson.Decode(ctx, b, buildLimits().Article)
		if err != nil {
			t.Fatal(err)
		}
		p, err := htmlterms.Build(ctx, a, buildLimits().Preparation)
		if err != nil {
			t.Fatal(err)
		}
		corpus.Editions[0].Passages = append(corpus.Editions[0].Passages, p.Fields...)
	}
	for _, raw := range []string{"retry", "unrelated", "retry unrelated", "missing retry", "words safely", "safely safely retry", "pods unrelated", "absent", "retry safely", "safely retry"} {
		p, err := textterms.Prepare(ctx, raw, buildLimits().Preparation.Field)
		if err != nil {
			t.Fatal(err)
		}
		q := termsearch.Query{TermsVersion: p.Version, Edition: o.index.ID(), Terms: p.Terms}
		baseline, err := o.index.Search(ctx, q, queryLimits())
		if err != nil {
			t.Fatal(err)
		}
		got, _, _, err := rankAny(ctx, o, q, baseline)
		if err != nil {
			t.Fatal(err)
		}
		terms := slices.Clone(q.Terms)
		slices.Sort(terms)
		terms = slices.Compact(terms)
		scores := make([]float64, len(o.positions))
		// Existing exact-scan BM25 is independent of counted posting execution.
		for _, term := range terms {
			one := q
			one.Terms = []string{term}
			result, err := termsearch.RankBM25(ctx, corpus, one, queryLimits())
			if err != nil {
				t.Fatal(err)
			}
			for _, h := range result.Matches {
				scores[h.Passage] += h.Score
			}
		}
		var want []termsearch.ScoredPassage
		for pos, field := range corpus.Editions[0].Passages {
			eligible := false
			for _, term := range terms {
				if slices.Contains(field.Body, term) || slices.Contains(field.Headings, term) || slices.Contains(field.Labels, term) {
					eligible = true
				}
			}
			if eligible {
				want = append(want, termsearch.ScoredPassage{Passage: pos, Score: scores[pos]})
			}
		}
		slices.SortFunc(want, func(a, b termsearch.ScoredPassage) int {
			if c := cmp.Compare(b.Score, a.Score); c != 0 {
				return c
			}
			return cmp.Compare(a.Passage, b.Passage)
		})
		if !slices.Equal(want, got.Matches) {
			t.Fatalf("scan union/scores differ for %q: got%+v want%+v", raw, got.Matches, want)
		}
	}
	q := termsearch.Query{TermsVersion: textterms.Version, Edition: o.index.ID(), Terms: []string{"retry"}}
	baseline, err := o.index.Search(ctx, q, queryLimits())
	if err != nil {
		t.Fatal(err)
	}
	damaged := baseline
	damaged.Matches = slices.Clone(baseline.Matches)
	damaged.Matches[0].Score++
	if _, _, _, err := rankAny(ctx, o, q, damaged); err == nil {
		t.Fatal("baseline score mismatch accepted")
	}
	dead, stop := context.WithCancel(ctx)
	stop()
	if got, _, _, err := rankAny(dead, o, q, baseline); !errors.Is(err, context.Canceled) || len(got.Matches) != 0 {
		t.Fatal("cancelled comparison returned results")
	}
}

func TestEvidenceKeepsNoticeLabelsOutsideQuote(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	a, err := htmlarticle.Extract(ctx, []byte(`<main class="fixture"><h1>Pods</h1><p>Intro.</p><div class="notice" title="FeatureGate"><p>Enabled by default.</p></div></main>`), htmlarticle.Profile{ID: "fixture/v1", ArticleClass: "fixture", NoticeClasses: []string{"notice"}}, extractLimits())
	if err != nil {
		t.Fatal(err)
	}
	body, err := htmlarticlejson.Encode(ctx, a, buildLimits().Article)
	if err != nil {
		t.Fatal(err)
	}
	index, err := htmledition.Build(ctx, "one", [][]byte{body}, buildLimits())
	if err != nil {
		t.Fatal(err)
	}
	o := openedEdition{index: index, bodies: [][]byte{body}, docs: []document{{Path: "notice.md", ArticleSHA256: hexHash(body)}}, positions: map[location]int{}}
	article, err := index.Article(0)
	if err != nil {
		t.Fatal(err)
	}
	for pos := range article.Passages {
		loc, err := index.Location(pos)
		if err != nil {
			t.Fatal(err)
		}
		o.positions[location{"notice.md", loc.Body.First, loc.Body.After}] = pos
	}
	base, trial, err := searchPair(ctx, o, "notice", "keywords", "FeatureGate", nil, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []caseReport{base, trial} {
		if len(c.Top) != 1 || c.Top[0].Quote != "Enabled by default." || !slices.Equal(c.Top[0].Headings, []string{"Pods"}) || !slices.Equal(c.Top[0].Labels, []string{"FeatureGate"}) {
			t.Fatalf("label-only match evidence changed: %+v", c)
		}
	}
}
