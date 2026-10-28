package termsearch

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"math/rand"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
)

func indexLimits() BuildLimits {
	return BuildLimits{MaxEditions: 10, MaxPassages: 1000, MaxTerms: 100000, MaxInputBytes: 1 << 20, MaxVocabulary: 10000, MaxPostings: 100000, MaxKeyBytes: 1 << 20}
}
func compareIndex(t *testing.T, ctx context.Context, c Corpus, q Query) {
	t.Helper()
	x, err := NewIndex(ctx, c, indexLimits())
	if err != nil {
		t.Fatal(err)
	}
	got, err := x.MatchAll(ctx, q, testLimits())
	want, werr := MatchAll(ctx, c, q, testLimits())
	if err != nil || werr != nil || !reflect.DeepEqual(got, want) {
		t.Fatal("matches", got, err, want, werr)
	}
	ranked, err := x.RankBM25(ctx, q, testLimits())
	expected, eerr := RankBM25(ctx, c, q, testLimits())
	if err != nil || eerr != nil || !reflect.DeepEqual(ranked, expected) {
		t.Fatal("ranked", ranked, err, expected, eerr)
	}
	// The independent formula uses Log instead of Log1p and different parentheses;
	// tiny rounding differences can reorder nearly tied scores. Compare its values
	// by passage, while the canonical scan above checks exact scores AND ordering.
	independent := oracleBM25(c, q).Matches
	actual := slices.Clone(ranked.Matches)
	byPassage := func(a, b ScoredPassage) int { return cmp.Compare(a.Passage, b.Passage) }
	slices.SortFunc(actual, byPassage)
	slices.SortFunc(independent, byPassage)
	assertScores(t, actual, independent)
}
func TestIndexMatchesScansAndFieldCountOracle(t *testing.T) {
	ctx := testContext(t)
	c := input()
	for _, edition := range []string{"1.35", "1.36", "empty", "missing"} {
		for _, terms := range [][]string{{"default"}, {"containerrestartrules", "default"}, {"default", "default", "rules"}, {"restart", "containerrestartrules", "rules"}, {"absent"}} {
			compareIndex(t, ctx, c, Query{TermsVersion: c.TermsVersion, Edition: edition, Terms: terms})
		}
	}
	x, err := NewIndex(ctx, c, indexLimits())
	if err != nil {
		t.Fatal(err)
	}
	for _, edition := range c.Editions {
		got := x.data.editions[edition.ID]
		vocabulary := map[string]bool{}
		total := 0
		for pi, p := range edition.Passages {
			length := len(p.Body) + len(p.Headings) + len(p.Labels)
			total += length
			if got.lengths[pi] != length {
				t.Fatal("length")
			}
			for _, field := range [3][]string{p.Body, p.Headings, p.Labels} {
				for _, term := range field {
					vocabulary[term] = true
				}
			}
		}
		if len(got.terms) != len(vocabulary) || got.totalLength != total {
			t.Fatal("statistics")
		}
		for term := range vocabulary {
			var expected []posting
			for pi, p := range edition.Passages {
				row := posting{passage: pi}
				for _, s := range p.Body {
					if s == term {
						row.body++
					}
				}
				for _, s := range p.Headings {
					if s == term {
						row.headings++
					}
				}
				for _, s := range p.Labels {
					if s == term {
						row.labels++
					}
				}
				if row.frequency() > 0 {
					expected = append(expected, row)
				}
			}
			if !slices.Equal(got.terms[term].rows, expected) {
				t.Fatal(term, got.terms[term].rows, expected)
			}
		}
	}
}
func TestIndexFinalIntersectionLimitAndTieOrder(t *testing.T) {
	c := Corpus{TermsVersion: "v1", Editions: []Edition{{ID: "chosen", Passages: []Fields{
		{Body: []string{"a"}}, {Body: []string{"a", "b"}}, {Body: []string{"a", "b"}}, {Body: []string{"b"}},
	}}}}
	ctx := testContext(t)
	x, err := NewIndex(ctx, c, indexLimits())
	if err != nil {
		t.Fatal(err)
	}
	l := testLimits()
	l.MaxMatches = 2
	q := Query{TermsVersion: "v1", Edition: "chosen", Terms: []string{"b", "a", "b"}}
	got, err := x.MatchAll(ctx, q, l)
	if err != nil || !slices.Equal(got.Passages, []int{1, 2}) {
		t.Fatal(got, err)
	}
	r, err := x.RankBM25(ctx, q, l)
	if err != nil || len(r.Matches) != 2 || r.Matches[0].Passage != 1 || r.Matches[1].Passage != 2 || r.Matches[0].Score != r.Matches[1].Score {
		t.Fatal(r, err)
	}
	for _, terms := range [][]string{{"a", "b"}, {"b", "a"}, {"a", "a", "b"}} {
		q.Terms = terms
		again, err := x.RankBM25(ctx, q, l)
		if err != nil || !reflect.DeepEqual(again, r) {
			t.Fatal(again, err)
		}
	}
	q.Terms = []string{"a"}
	if got, err := x.MatchAll(ctx, q, l); !errors.Is(err, ErrLimit) || !reflect.DeepEqual(got, Result{}) {
		t.Fatal(got, err)
	}
}
func TestIndexOwnsInputAndIndependentResults(t *testing.T) {
	c, q := rankingInput()
	ctx := testContext(t)
	before, _ := json.Marshal(c)
	x, err := NewIndex(ctx, c, indexLimits())
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(c)
	if !slices.Equal(before, after) {
		t.Fatal("mutated build input")
	}
	expected, err := x.RankBM25(ctx, q, testLimits())
	if err != nil {
		t.Fatal(err)
	}
	c.TermsVersion = "changed"
	c.Editions[0].ID = "changed"
	for _, p := range c.Editions[0].Passages {
		for _, field := range [3][]string{p.Body, p.Headings, p.Labels} {
			for i := range field {
				field[i] = "changed"
			}
		}
	}
	again, err := x.RankBM25(ctx, q, testLimits())
	if err != nil || !reflect.DeepEqual(again, expected) {
		t.Fatal(again, err)
	}
	again.Matches[0].Passage = 999
	ids, err := x.MatchAll(ctx, q, testLimits())
	if err != nil {
		t.Fatal(err)
	}
	ids.Passages[0] = 999
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 10 {
				r, err := x.RankBM25(ctx, q, testLimits())
				if err != nil || !reflect.DeepEqual(r, expected) {
					t.Error(r, err)
				}
			}
		})
	}
	wg.Wait()
}
func measuredBuildLimits(c Corpus) BuildLimits {
	l := BuildLimits{MaxEditions: len(c.Editions), MaxInputBytes: len(c.TermsVersion), MaxKeyBytes: len(c.TermsVersion)}
	for _, e := range c.Editions {
		l.MaxPassages += len(e.Passages)
		l.MaxInputBytes += len(e.ID)
		l.MaxKeyBytes += len(e.ID)
		vocab := map[string]bool{}
		for _, p := range e.Passages {
			words := map[string]bool{}
			for _, field := range [3][]string{p.Body, p.Headings, p.Labels} {
				for _, s := range field {
					l.MaxTerms++
					l.MaxInputBytes += len(s)
					words[s] = true
					vocab[s] = true
				}
			}
			l.MaxPostings += len(words)
		}
		l.MaxVocabulary += len(vocab)
		for s := range vocab {
			l.MaxKeyBytes += len(s)
		}
	}
	return l
}
func TestIndexAdmissionAndStorageLimits(t *testing.T) {
	c, q := rankingInput()
	ctx := testContext(t)
	exact := measuredBuildLimits(c)
	if _, err := NewIndex(ctx, c, exact); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"MaxEditions", "MaxPassages", "MaxTerms", "MaxInputBytes", "MaxVocabulary", "MaxPostings", "MaxKeyBytes"} {
		low := exact
		v := reflect.ValueOf(&low).Elem().FieldByName(field)
		v.SetInt(v.Int() - 1)
		// MaxEditions=1 decreased to zero is invalid configuration, not overflow.
		x, err := NewIndex(ctx, c, low)
		if x != nil || !(errors.Is(err, ErrLimit) || errors.Is(err, ErrInvalid) && v.Int() == 0) {
			t.Fatal(field, err)
		}
		zero := indexLimits()
		reflect.ValueOf(&zero).Elem().FieldByName(field).SetInt(0)
		if x, err := NewIndex(ctx, c, zero); x != nil || !errors.Is(err, ErrInvalid) {
			t.Fatal(field, err)
		}
	}
	// Cached admission includes all original occurrence bytes and counts.
	c.Editions = append(c.Editions, Edition{ID: "other", Passages: []Fields{{Body: []string{"large-other-input"}}}})
	x, err := NewIndex(ctx, c, indexLimits())
	if err != nil {
		t.Fatal(err)
	}
	original := measuredBuildLimits(c)
	l := testLimits()
	l.MaxEditions = original.MaxEditions
	l.MaxPassages = original.MaxPassages
	l.MaxTerms = original.MaxTerms
	l.MaxInputBytes = original.MaxInputBytes + len(q.TermsVersion) + len(q.Edition)
	for _, s := range q.Terms {
		l.MaxInputBytes += len(s)
	}
	if _, err := x.RankBM25(ctx, q, l); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"MaxEditions", "MaxPassages", "MaxTerms", "MaxInputBytes", "MaxQueryTerms"} {
		low := l
		if field == "MaxQueryTerms" {
			low.MaxQueryTerms = len(q.Terms) - 1
		} else {
			v := reflect.ValueOf(&low).Elem().FieldByName(field)
			v.SetInt(v.Int() - 1)
		}
		got, err := x.MatchAll(ctx, q, low)
		if !errors.Is(err, ErrLimit) || !reflect.DeepEqual(got, Result{}) {
			t.Fatal(field, got, err)
		}
	}
}
func TestIndexRejectsMalformedInputAndQuery(t *testing.T) {
	ctx := testContext(t)
	c, q := rankingInput()
	for _, mutate := range []func(*Corpus){func(c *Corpus) { c.TermsVersion = "" }, func(c *Corpus) { c.Editions = append(c.Editions, c.Editions[0]) }, func(c *Corpus) { c.Editions[0].ID = "" }, func(c *Corpus) { c.Editions[0].Passages[0].Body = []string{""} }, func(c *Corpus) { c.Editions[0].Passages[0].Labels = []string{"nul\x00term"} }, func(c *Corpus) { c.Editions[0].Passages[0].Headings = []string{string([]byte{0xff})} }} {
		b, _ := json.Marshal(c)
		var bad Corpus
		_ = json.Unmarshal(b, &bad)
		mutate(&bad)
		if x, err := NewIndex(ctx, bad, indexLimits()); x != nil || !errors.Is(err, ErrInvalid) {
			t.Fatal(err)
		}
	}
	x, err := NewIndex(ctx, c, indexLimits())
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []Query{{TermsVersion: q.TermsVersion, Edition: q.Edition}, {TermsVersion: q.TermsVersion, Edition: "", Terms: q.Terms}, {TermsVersion: q.TermsVersion, Edition: q.Edition, Terms: []string{""}}, {TermsVersion: q.TermsVersion, Edition: q.Edition, Terms: []string{"bad\x00term"}}, {TermsVersion: q.TermsVersion, Edition: q.Edition, Terms: []string{string([]byte{0xff})}}} {
		if got, err := x.MatchAll(ctx, bad, testLimits()); !errors.Is(err, ErrInvalid) || !reflect.DeepEqual(got, Result{}) {
			t.Fatal(got, err)
		}
	}
	q.TermsVersion = "other"
	if _, err := x.MatchAll(ctx, q, testLimits()); !errors.Is(err, ErrIncompatible) {
		t.Fatal(err)
	}
	for _, ctx := range []context.Context{nil, context.Background()} {
		if x, err := NewIndex(ctx, c, indexLimits()); x != nil || err == nil {
			t.Fatal(err)
		}
		if _, err := x.RankBM25(ctx, q, testLimits()); err == nil {
			t.Fatal(err)
		}
	}
	for _, zero := range []*Index{nil, {}} {
		if _, err := zero.MatchAll(ctx, q, testLimits()); !errors.Is(err, ErrInvalid) {
			t.Fatal(err)
		}
	}
}
func TestGeneratedIndexesExactlyMatchScan(t *testing.T) {
	rng := rand.New(rand.NewSource(331))
	ctx := testContext(t)
	vocab := []string{"a", "b", "C++", "世界", "ContainerRestartRules", "foo-bar"}
	for range 300 {
		c := Corpus{TermsVersion: "fixture/v1", Editions: []Edition{{ID: "chosen"}, {ID: "other"}, {ID: "empty"}}}
		for e := 0; e < 2; e++ {
			for n := rng.Intn(12); n > 0; n-- {
				var p Fields
				for _, field := range []*[]string{&p.Body, &p.Headings, &p.Labels} {
					for n := rng.Intn(8); n > 0; n-- {
						*field = append(*field, vocab[rng.Intn(len(vocab))])
					}
				}
				c.Editions[e].Passages = append(c.Editions[e].Passages, p)
			}
		}
		q := Query{TermsVersion: c.TermsVersion, Edition: []string{"chosen", "other", "empty", "missing"}[rng.Intn(4)]}
		for n := 1 + rng.Intn(5); n > 0; n-- {
			q.Terms = append(q.Terms, append(vocab, "absent")[rng.Intn(len(vocab)+1)])
		}
		compareIndex(t, ctx, c, q)
	}
}
func TestIndexExactTermsAndEmptyCorpus(t *testing.T) {
	ctx := testContext(t)
	for _, c := range []Corpus{{TermsVersion: "v1"}, {TermsVersion: "v1", Editions: []Edition{{ID: "empty"}}}, {TermsVersion: "v1", Editions: []Edition{{ID: "e", Passages: []Fields{{Body: []string{"Go", "go", "é", "e\u0301", "a b", strings.Repeat("z", 100)}}}}}}} {
		for _, term := range []string{"Go", "GO", "é", "e\u0301", "a b", "a"} {
			compareIndex(t, ctx, c, Query{TermsVersion: c.TermsVersion, Edition: "e", Terms: []string{term}})
		}
	}
}

type indexInterrupt struct {
	context.Context
	cancel      context.CancelFunc
	calls, stop int
}

func (c *indexInterrupt) Err() error {
	c.calls++
	if c.stop > 0 && c.calls >= c.stop {
		c.cancel()
	}
	return c.Context.Err()
}
func TestIndexEveryObservedCancellationBoundary(t *testing.T) {
	ctx := testContext(t)
	c, q := rankingInput()
	x, err := NewIndex(ctx, c, indexLimits())
	if err != nil {
		t.Fatal(err)
	}
	for which, invoke := range []func(context.Context) error{
		func(ctx context.Context) error {
			x, err := NewIndex(ctx, c, indexLimits())
			if err != nil && x != nil {
				t.Fatal("partial index")
			}
			return err
		},
		func(ctx context.Context) error {
			r, err := x.MatchAll(ctx, q, testLimits())
			if err != nil && !reflect.DeepEqual(r, Result{}) {
				t.Fatal("partial match")
			}
			return err
		},
		func(ctx context.Context) error {
			r, err := x.RankBM25(ctx, q, testLimits())
			if err != nil && !reflect.DeepEqual(r, RankedResult{}) {
				t.Fatal("partial rank")
			}
			return err
		},
	} {
		parent, cancel := context.WithCancel(ctx)
		counter := &indexInterrupt{Context: parent, cancel: cancel}
		if err := invoke(counter); err != nil {
			t.Fatal(err)
		}
		cancel()
		for stop := 1; stop <= counter.calls; stop++ {
			parent, cancel := context.WithCancel(ctx)
			seam := &indexInterrupt{Context: parent, cancel: cancel, stop: stop}
			err := invoke(seam)
			cancel()
			if !errors.Is(err, context.Canceled) {
				t.Fatal(which, stop, err)
			}
		}
	}
}
func FuzzIndexAgainstScan(f *testing.F) {
	f.Add([]byte{0, 1, 2, 3, 255, 1, 3})
	f.Add([]byte{})
	f.Add([]byte("888888000000000100000001000000020000010000000000210111112"))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 96 {
			t.Skip()
		}
		c := Corpus{TermsVersion: "fuzz/v1", Editions: []Edition{{ID: "a", Passages: make([]Fields, 8)}, {ID: "b", Passages: make([]Fields, 8)}, {ID: "empty"}}}
		for i, b := range data {
			p := &c.Editions[int(b)%2].Passages[i%8]
			field := []*[]string{&p.Body, &p.Headings, &p.Labels}[int(b)%3]
			*field = append(*field, string(rune('a'+b%6)))
		}
		q := Query{TermsVersion: c.TermsVersion, Edition: "a", Terms: []string{"a"}}
		if len(data) > 0 {
			q.Edition = []string{"a", "b", "empty", "missing"}[int(data[0])%4]
			for _, b := range data[:min(6, len(data))] {
				q.Terms = append(q.Terms, string(rune('a'+b%7)))
			}
		}
		compareIndex(t, testContext(t), c, q)
	})
}
