package textterms_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/sidsharma96/concordance/internal/termsearch"
	"github.com/sidsharma96/concordance/internal/textterms"
)

func TestPreparedTextWithExactMatching(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	prepare := func(text string) textterms.Prepared {
		t.Helper()
		got, err := textterms.Prepare(ctx, text, textterms.Limits{MaxInputBytes: 1024, MaxTerms: 100, MaxTermBytes: 100, MaxOutputBytes: 1024})
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	// Synthetic field choices, not an adopted retained-article projection.
	cpp, c := prepare("Configure the C++ compiler."), prepare("Configure the C compiler.")
	label := prepare("ContainerRestartRules")
	corpus := termsearch.Corpus{TermsVersion: cpp.Version, Editions: []termsearch.Edition{
		{ID: "1.35", Passages: []termsearch.Fields{
			{Body: cpp.Terms, Headings: prepare("readiness-probe v1.35").Terms},
			{Body: c.Terms},
			{Body: prepare("enabled by default").Terms, Labels: label.Terms},
		}},
		{ID: "1.36", Passages: []termsearch.Fields{{Body: prepare("readiness probe").Terms}}},
	}}
	limits := termsearch.Limits{MaxEditions: 2, MaxPassages: 10, MaxTerms: 100, MaxQueryTerms: 10, MaxInputBytes: 4096, MaxMatches: 10}
	for _, tc := range []struct {
		text, edition string
		want          []int
	}{
		{"C++", "1.35", []int{0}},
		{"C", "1.35", []int{1}},
		{"readiness-probe V1.35", "1.35", []int{0}},
		{"readiness", "1.35", nil},
		{"ContainerRestartRules default", "1.35", []int{2}},
		{"restart rules", "1.35", nil},
		{"readiness probe", "1.36", []int{0}},
	} {
		q := prepare(tc.text)
		got, err := termsearch.MatchAll(ctx, corpus, termsearch.Query{TermsVersion: q.Version, Edition: tc.edition, Terms: q.Terms}, limits)
		if err != nil || !got.EditionFound || !reflect.DeepEqual(got.Passages, tc.want) {
			t.Fatal(tc, got, err)
		}
	}
	q := prepare("C++")
	_, err := termsearch.MatchAll(ctx, corpus, termsearch.Query{TermsVersion: q.Version + "/changed", Edition: "1.35", Terms: q.Terms}, limits)
	if !errors.Is(err, termsearch.ErrIncompatible) {
		t.Fatal("mismatched preparation accepted", err)
	}
	q = prepare("?!")
	_, err = termsearch.MatchAll(ctx, corpus, termsearch.Query{TermsVersion: q.Version, Edition: "1.35", Terms: q.Terms}, limits)
	if !errors.Is(err, termsearch.ErrInvalid) {
		t.Fatal("empty prepared query accepted", err)
	}
}
