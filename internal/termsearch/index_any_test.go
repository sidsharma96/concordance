package termsearch

import (
	"cmp"
	"context"
	"errors"
	"math/rand"
	"reflect"
	"slices"
	"sync"
	"testing"
)

// Keep the scan/single-term composition as an oracle for the posting union.
func scanAny(t *testing.T, c Corpus, q Query) []ScoredPassage {
	t.Helper()
	terms := slices.Clone(q.Terms)
	slices.Sort(terms)
	terms = slices.Compact(terms)
	scores := map[int]float64{}
	for _, term := range terms {
		one := q
		one.Terms = []string{term}
		r, err := RankBM25(testContext(t), c, one, testLimits())
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range r.Matches {
			scores[row.Passage] += row.Score
		}
	}
	var out []ScoredPassage
	for position, score := range scores {
		out = append(out, ScoredPassage{position, score})
	}
	slices.SortFunc(out, func(a, b ScoredPassage) int {
		if n := cmp.Compare(b.Score, a.Score); n != 0 {
			return n
		}
		return cmp.Compare(a.Passage, b.Passage)
	})
	return out
}

func TestAnyBM25MatchesIndependentSingleTermScans(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	words := []string{"alpha", "beta", "gamma", "delta"}
	for trial := range 200 {
		c := Corpus{TermsVersion: "fixture/v1", Editions: []Edition{{ID: "one"}, {ID: "other", Passages: []Fields{{Body: []string{"alpha", "alpha"}}}}, {ID: "empty"}}}
		for range rng.Intn(40) {
			p := Fields{}
			for _, field := range []*[]string{&p.Body, &p.Headings, &p.Labels} {
				for range rng.Intn(7) {
					*field = append(*field, words[rng.Intn(len(words))])
				}
			}
			c.Editions[0].Passages = append(c.Editions[0].Passages, p)
		}
		x, err := NewIndex(testContext(t), c, indexLimits())
		if err != nil {
			t.Fatal(err)
		}
		for _, edition := range []string{"one", "other", "empty", "absent"} {
			q := Query{TermsVersion: c.TermsVersion, Edition: edition, Terms: []string{"beta", "absent", "alpha", "beta"}}
			got, err := x.RankBM25Any(testContext(t), q, testLimits())
			if err != nil || got.EditionFound != (edition != "absent") || got.RankingVersion != AnyBM25Version || !slices.Equal(got.Matches, scanAny(t, c, q)) {
				t.Fatal("exact scan/order mismatch", trial, edition, got, err)
			}
			slices.Reverse(q.Terms)
			again, err := x.RankBM25Any(testContext(t), q, testLimits())
			if err != nil || !reflect.DeepEqual(got, again) {
				t.Fatal("query order/duplicates changed scores", err)
			}
		}
	}
	t.Log("200 generated corpora: exact any-word scores/order match single-term scans across selected, other, empty and absent editions")
}

func TestAnyBM25AdmissionAndConcurrentOwnership(t *testing.T) {
	c := Corpus{TermsVersion: "v", Editions: []Edition{{ID: "one", Passages: []Fields{{Body: []string{"a"}}, {Body: []string{"b"}}, {Body: []string{"a", "b"}}, {}}}}}
	x, err := NewIndex(testContext(t), c, indexLimits())
	if err != nil {
		t.Fatal(err)
	}
	q := Query{TermsVersion: "v", Edition: "one", Terms: []string{"b", "a", "missing"}}
	want, err := x.RankBM25Any(testContext(t), q, testLimits())
	if err != nil || len(want.Matches) != 3 {
		t.Fatal(want, err)
	}
	all, err := x.RankBM25(testContext(t), q, testLimits())
	if err != nil || len(all.Matches) != 0 {
		t.Fatal("match-all changed", all, err)
	}
	for _, change := range []func(*Limits){
		func(l *Limits) { l.MaxMatches = 2 },
		func(l *Limits) { l.MaxQueryTerms = 2 },
		func(l *Limits) { l.MaxPassages = 3 },
		func(l *Limits) { l.MaxTerms = 3 },
		func(l *Limits) { l.MaxInputBytes = 3 },
	} {
		l := testLimits()
		change(&l)
		got, err := x.RankBM25Any(testContext(t), q, l)
		if !errors.Is(err, ErrLimit) || !reflect.DeepEqual(got, RankedResult{}) {
			t.Fatal("partial/budget bypass", got, err)
		}
	}
	for _, bad := range []Query{{"v", "one", nil}, {"v", "one", []string{"a", ""}}, {"different", "one", []string{"a"}}} {
		if got, err := x.RankBM25Any(testContext(t), bad, testLimits()); err == nil || !reflect.DeepEqual(got, RankedResult{}) {
			t.Fatal("invalid query admitted", got, err)
		}
	}
	ctx, cancel := context.WithCancel(testContext(t))
	cancel()
	if got, err := x.RankBM25Any(ctx, q, testLimits()); !errors.Is(err, context.Canceled) || !reflect.DeepEqual(got, RankedResult{}) {
		t.Fatal("canceled result", got, err)
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 20 {
				got, err := x.RankBM25Any(testContext(t), q, testLimits())
				if err != nil || !reflect.DeepEqual(got, want) {
					t.Error("concurrent query mutated state", err)
					return
				}
				got.Matches[0] = ScoredPassage{}
			}
		})
	}
	wg.Wait()
}
