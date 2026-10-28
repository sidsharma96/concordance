package termsearch

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"
)

// Synthetic warm-query comparison; construction is measured separately. Input is
// identical and results are checked before timing. This is not a corpus relevance,
// retained-heap, cold-open or provider benchmark.
func benchmarkIndexInput() (Corpus, BuildLimits, Limits) {
	c := Corpus{TermsVersion: "bench/v1", Editions: []Edition{{ID: "chosen", Passages: make([]Fields, 4096)}}}
	for i := range c.Editions[0].Passages {
		p := &c.Editions[0].Passages[i]
		p.Body = []string{"common"}
		for j := range 20 {
			p.Body = append(p.Body, fmt.Sprintf("word%d", (i+j)%32))
		}
		p.Headings = []string{"guide", "common"}
		if i%101 == 0 {
			p.Labels = []string{"rare"}
		}
		if i == 4095 {
			p.Body = append(p.Body, "last")
		}
	}
	b := BuildLimits{MaxEditions: 2, MaxPassages: 10000, MaxTerms: 1000000, MaxInputBytes: 16 << 20, MaxVocabulary: 100000, MaxPostings: 1000000, MaxKeyBytes: 16 << 20}
	q := Limits{MaxEditions: 2, MaxPassages: 10000, MaxTerms: 1000000, MaxQueryTerms: 10, MaxInputBytes: 16 << 20, MaxMatches: 10000}
	return c, b, q
}
func BenchmarkCountedPostings(b *testing.B) {
	c, build, queryLimits := benchmarkIndexInput()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	index, err := NewIndex(ctx, c, build)
	if err != nil {
		b.Fatal(err)
	}
	b.Run("build", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, err := NewIndex(ctx, c, build); err != nil {
				b.Fatal(err)
			}
		}
	})
	for _, tc := range []struct {
		name  string
		terms []string
	}{{"sparse", []string{"rare", "common"}}, {"dense", []string{"common"}}, {"last", []string{"last", "common"}}, {"miss", []string{"missing", "common"}}} {
		q := Query{TermsVersion: c.TermsVersion, Edition: "chosen", Terms: tc.terms}
		expected, err := RankBM25(ctx, c, q, queryLimits)
		if err != nil {
			b.Fatal(err)
		}
		got, err := index.RankBM25(ctx, q, queryLimits)
		if err != nil || !reflect.DeepEqual(got, expected) {
			b.Fatal("different results", err)
		}
		b.Run(tc.name, func(b *testing.B) {
			b.Run("scan", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					if _, err := RankBM25(ctx, c, q, queryLimits); err != nil {
						b.Fatal(err)
					}
				}
			})
			b.Run("index", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					if _, err := index.RankBM25(ctx, q, queryLimits); err != nil {
						b.Fatal(err)
					}
				}
			})
		})
	}
}
