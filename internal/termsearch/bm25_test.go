package termsearch

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"testing"
)

func rankingInput() (Corpus, Query) {
	c := Corpus{TermsVersion: "fixture/v1", Editions: []Edition{{ID: "chosen", Passages: []Fields{
		{Body: []string{"restart", "default", "always", "x"}},
		{Body: []string{"restart", "default", "default", "x", "x", "x", "x", "x"}},
		{Body: []string{"restart", "x", "x", "x"}},
		{},
	}}}}
	return c, Query{TermsVersion: c.TermsVersion, Edition: "chosen", Terms: []string{"restart", "default"}}
}

func TestBM25WorkedExample(t *testing.T) {
	c, q := rankingInput()
	got, err := RankBM25(testContext(t), c, q, testLimits())
	if err != nil || !got.EditionFound || got.RankingVersion != BM25Version || len(got.Matches) != 2 {
		t.Fatal(got, err)
	}
	// Independently evaluated arithmetic: N=4, total length=16, df=(3,2).
	want := []ScoredPassage{{0, 1.0498221244986776}, {1, 0.9969894207053518}}
	assertScores(t, got.Matches, want)
	// No statistical influence from other editions, even one with many matches.
	other := Edition{ID: "other"}
	for range 40 {
		other.Passages = append(other.Passages, Fields{Labels: []string{"restart", "default", "default"}})
	}
	c.Editions = append([]Edition{other}, c.Editions...)
	for _, terms := range [][]string{{"default", "restart"}, {"restart", "default", "restart"}} {
		q.Terms = terms
		again, err := RankBM25(testContext(t), c, q, testLimits())
		if err != nil || !reflect.DeepEqual(again, got) {
			t.Fatal("edition leaked or query order/duplicates matter", again, err)
		}
	}
}

func TestBM25FieldsTiesAndOwnership(t *testing.T) {
	c, q := rankingInput()
	c.Editions[0].Passages = []Fields{
		{Body: []string{"default"}, Headings: []string{"restart"}},
		{Labels: []string{"restart", "default"}},
		{Body: []string{"restart", "default"}},
	}
	before, _ := json.Marshal([]any{c, q})
	got, err := RankBM25(testContext(t), c, q, testLimits())
	if err != nil || len(got.Matches) != 3 {
		t.Fatal(got, err)
	}
	for i, hit := range got.Matches {
		if hit.Passage != i || hit.Score != got.Matches[0].Score {
			t.Fatal("field weight or unstable tie", got)
		}
	}
	after, _ := json.Marshal([]any{c, q})
	if string(before) != string(after) {
		t.Fatal("input mutated")
	}
	got.Matches[0] = ScoredPassage{99, -1}
	again, err := RankBM25(testContext(t), c, q, testLimits())
	if err != nil || again.Matches[0].Passage != 0 || again.Matches[0].Score <= 0 {
		t.Fatal("output aliases state", again, err)
	}
	c.Editions[0].Passages[0].Headings[0] = "changed"
	if again.Matches[0].Passage != 0 {
		t.Fatal("output aliases input")
	}
}

func TestBM25SaturationAndLength(t *testing.T) {
	c, q := rankingInput()
	q.Terms = []string{"word"}
	c.Editions[0].Passages = []Fields{
		{Body: []string{"word", "x", "x", "x"}},
		{Body: []string{"word", "word", "x", "x"}},
		{Body: []string{"word", "word", "word", "x"}},
		{Body: []string{"word", "x", "x", "x", "x", "x", "x", "x"}},
	}
	r, err := RankBM25(testContext(t), c, q, testLimits())
	if err != nil {
		t.Fatal(err)
	}
	scores := make([]float64, 4)
	for _, hit := range r.Matches {
		scores[hit.Passage] = hit.Score
	}
	if !(scores[2] > scores[1] && scores[1] > scores[0] && scores[0] > scores[3]) {
		t.Fatal("frequency/length effects", scores)
	}
	if scores[2]-scores[1] >= scores[1]-scores[0] {
		t.Fatal("no diminishing reward", scores)
	}
}

func TestBM25NoMatchesAndFailures(t *testing.T) {
	c, q := rankingInput()
	c.Editions = append(c.Editions, Edition{ID: "empty"})
	for _, edition := range []string{"missing", "empty", "chosen"} {
		q.Edition = edition
		q.Terms = []string{"absent"}
		got, err := RankBM25(testContext(t), c, q, testLimits())
		want := RankedResult{EditionFound: edition != "missing", RankingVersion: BM25Version}
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatal(got, err, want)
		}
	}
	for _, kind := range []string{"bad term", "bad unselected", "incompatible", "limit", "no deadline", "canceled", "input limit"} {
		t.Run(kind, func(t *testing.T) {
			c, q := rankingInput()
			l := testLimits()
			ctx := testContext(t)
			want := ErrInvalid
			switch kind {
			case "bad term":
				q.Terms[0] = ""
			case "bad unselected":
				c.Editions = append(c.Editions, Edition{ID: "other", Passages: []Fields{{Body: []string{"\x00"}}}})
			case "incompatible":
				q.TermsVersion = "different"
				want = ErrIncompatible
			case "limit":
				l.MaxMatches = 1
				want = ErrLimit
			case "input limit":
				l.MaxTerms = 1
				want = ErrLimit
			case "no deadline":
				ctx = context.Background()
			case "canceled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
				want = context.Canceled
			}
			got, err := RankBM25(ctx, c, q, l)
			if !errors.Is(err, want) || !reflect.DeepEqual(got, RankedResult{}) {
				t.Fatal(got, err, want)
			}
		})
	}
}

func TestBM25CancellationAtEveryCheck(t *testing.T) {
	// Exercise statistics, rescan/scoring and final-after-sort boundaries too,
	// not only the delegated MatchAll phase. The test seam counts Err visits.
	for _, noMatches := range []bool{false, true} {
		c, q := rankingInput()
		if noMatches {
			q.Terms = []string{"absent"}
		}
		count := &countedContext{Context: testContext(t)}
		if _, err := RankBM25(count, c, q, testLimits()); err != nil {
			t.Fatal(err)
		}
		for stop := 1; stop <= count.calls; stop++ {
			ctx := &countedContext{Context: testContext(t), stop: stop}
			got, err := RankBM25(ctx, c, q, testLimits())
			if !errors.Is(err, context.Canceled) || !reflect.DeepEqual(got, RankedResult{}) {
				t.Fatalf("noMatches=%v stop=%d: %+v, %v", noMatches, stop, got, err)
			}
		}
	}
}

func assertScores(t *testing.T, got, want []ScoredPassage) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i, hit := range got {
		if hit.Passage != want[i].Passage || math.IsNaN(hit.Score) || math.IsInf(hit.Score, 0) || hit.Score <= 0 || math.Abs(hit.Score-want[i].Score) > 1e-12*math.Max(1, math.Abs(want[i].Score)) {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}
