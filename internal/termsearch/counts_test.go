package termsearch

import (
	"context"
	"errors"
	"math/rand/v2"
	"reflect"
	"testing"
)

func TestCountTransportGeneratedAgainstOriginalScan(t *testing.T) {
	ctx := testContext(t)
	r := rand.New(rand.NewPCG(71, 109))
	for range 200 {
		c := Corpus{TermsVersion: "fixture/v1", Editions: []Edition{{ID: "chosen"}, {ID: "other"}}}
		for ei := range c.Editions {
			for n := r.IntN(20); n > 0; n-- {
				p := Fields{}
				for _, dst := range []*[]string{&p.Body, &p.Headings, &p.Labels} {
					for m := r.IntN(8); m > 0; m-- {
						*dst = append(*dst, string(rune('a'+r.IntN(6))))
					}
				}
				c.Editions[ei].Passages = append(c.Editions[ei].Passages, p)
			}
		}
		x, err := NewIndex(ctx, c, indexLimits())
		if err != nil {
			t.Fatal(err)
		}
		counts, found, err := x.ExportEdition(ctx, "chosen", indexLimits())
		if err != nil || !found {
			t.Fatal(found, err)
		}
		restored, err := NewCountIndex(ctx, counts, indexLimits())
		if err != nil {
			t.Fatal(err)
		}
		selected := Corpus{TermsVersion: c.TermsVersion, Editions: c.Editions[:1]}
		size, err := admitIndex(ctx, selected, indexLimits())
		if err != nil || restored.data.input != size {
			t.Fatal("reconstructed admission", restored.data.input, size, err)
		}
		for _, words := range [][]string{{"a"}, {"a", "b", "a"}, {"e", "c"}, {"missing"}} {
			q := Query{TermsVersion: c.TermsVersion, Edition: "chosen", Terms: words}
			got, err := restored.RankBM25(ctx, q, testLimits())
			want, werr := RankBM25(ctx, selected, q, testLimits())
			if err != nil || werr != nil || !reflect.DeepEqual(got, want) {
				t.Fatal(got, want, err, werr)
			}
		}
		// Mutating either exported transport must not modify either immutable index.
		before, _, err := restored.ExportEdition(ctx, "chosen", indexLimits())
		if err != nil {
			t.Fatal(err)
		}
		for i := range counts.Lengths {
			counts.Lengths[i] = 99
		}
		for i := range counts.Words {
			counts.Words[i].Term = "changed"
			clear(counts.Words[i].Rows)
		}
		after, _, err := restored.ExportEdition(ctx, "chosen", indexLimits())
		if err != nil || !reflect.DeepEqual(before, after) {
			t.Fatal("borrowed count aliases", err)
		}
		original, _, err := x.ExportEdition(ctx, "chosen", indexLimits())
		if err != nil || !reflect.DeepEqual(before, original) {
			t.Fatal("export aliases original", err)
		}
	}
}
func validCounts() EditionCounts {
	return EditionCounts{TermsVersion: "v1", Edition: "e", Lengths: []int{3, 1, 0}, TotalTerms: 4, Words: []CountedWord{{Term: "a", Rows: []FieldCount{{Passage: 0, Body: 1, Headings: 1}, {Passage: 1, Labels: 1}}}, {Term: "b", Rows: []FieldCount{{Passage: 0, Body: 1}}}}}
}
func TestCountTransportMalformedAndBudgets(t *testing.T) {
	ctx := testContext(t)
	cases := map[string]func(*EditionCounts){
		"negative count":     func(c *EditionCounts) { c.Words[0].Rows[0].Body = -1 },
		"overflow sum":       func(c *EditionCounts) { c.Words[0].Rows[0].Body = int(^uint(0) >> 1); c.Words[0].Rows[0].Labels = 1 },
		"zero row":           func(c *EditionCounts) { c.Words[0].Rows[0] = FieldCount{Passage: 0} },
		"duplicate position": func(c *EditionCounts) { c.Words[0].Rows[1].Passage = 0 },
		"negative position":  func(c *EditionCounts) { c.Words[0].Rows[0].Passage = -1 },
		"outside position":   func(c *EditionCounts) { c.Words[0].Rows[1].Passage = 3 },
		"duplicate word":     func(c *EditionCounts) { c.Words[1].Term = "a" },
		"unsorted words":     func(c *EditionCounts) { c.Words[0], c.Words[1] = c.Words[1], c.Words[0] },
		"empty list":         func(c *EditionCounts) { c.Words[0].Rows = nil },
		"bad UTF8":           func(c *EditionCounts) { c.Words[0].Term = "\xff" },
		"NUL":                func(c *EditionCounts) { c.TermsVersion = "v\x00" },
		"wrong length":       func(c *EditionCounts) { c.Lengths[2] = 1 },
		"negative length":    func(c *EditionCounts) { c.Lengths[2] = -1 },
		"wrong total":        func(c *EditionCounts) { c.TotalTerms = 3 },
		"negative total":     func(c *EditionCounts) { c.TotalTerms = -1 },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			c := validCounts()
			mutate(&c)
			x, err := NewCountIndex(ctx, c, indexLimits())
			if err == nil || x != nil {
				t.Fatal("accepted malformed", err)
			}
		})
	}
	for name, mutate := range map[string]func(*BuildLimits){
		"passages": func(l *BuildLimits) { l.MaxPassages = 2 }, "terms": func(l *BuildLimits) { l.MaxTerms = 3 }, "vocabulary": func(l *BuildLimits) { l.MaxVocabulary = 1 }, "postings": func(l *BuildLimits) { l.MaxPostings = 2 }, "keys": func(l *BuildLimits) { l.MaxKeyBytes = 4 }, "occurrence bytes": func(l *BuildLimits) { l.MaxInputBytes = 6 },
	} {
		t.Run(name, func(t *testing.T) {
			l := indexLimits()
			mutate(&l)
			if x, err := NewCountIndex(ctx, validCounts(), l); x != nil || !errors.Is(err, ErrLimit) {
				t.Fatal(x, err)
			}
			x, err := NewCountIndex(ctx, validCounts(), indexLimits())
			if err != nil {
				t.Fatal(err)
			}
			c, found, err := x.ExportEdition(ctx, "e", l)
			if !errors.Is(err, ErrLimit) || found || !reflect.DeepEqual(c, EditionCounts{}) {
				t.Fatal(c, found, err)
			}
		})
	}
	x, err := NewCountIndex(ctx, validCounts(), indexLimits())
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []context.Context{nil, context.Background()} {
		if got, err := NewCountIndex(bad, validCounts(), indexLimits()); err == nil || got != nil {
			t.Fatal(err)
		}
		if _, found, err := x.ExportEdition(bad, "e", indexLimits()); err == nil || found {
			t.Fatal(err)
		}
	}
	if _, found, err := x.ExportEdition(ctx, "missing", indexLimits()); err != nil || found {
		t.Fatal(err)
	}
}
func TestCountTransportEveryCancellationBoundary(t *testing.T) {
	ctx := testContext(t)
	x, err := NewCountIndex(ctx, validCounts(), indexLimits())
	if err != nil {
		t.Fatal(err)
	}
	for _, invoke := range []func(context.Context) error{
		func(ctx context.Context) error {
			x, err := NewCountIndex(ctx, validCounts(), indexLimits())
			if err != nil && x != nil {
				t.Fatal("partial")
			}
			return err
		},
		func(ctx context.Context) error {
			c, found, err := x.ExportEdition(ctx, "e", indexLimits())
			if err != nil && (found || !reflect.DeepEqual(c, EditionCounts{})) {
				t.Fatal("partial")
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
			err := invoke(&indexInterrupt{Context: parent, cancel: cancel, stop: stop})
			cancel()
			if !errors.Is(err, context.Canceled) {
				t.Fatal(stop, err)
			}
		}
	}
}
