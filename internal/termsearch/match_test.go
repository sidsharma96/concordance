package termsearch

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"
)

func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}
func testLimits() Limits {
	return Limits{MaxEditions: 10, MaxPassages: 100, MaxTerms: 10000, MaxQueryTerms: 50, MaxInputBytes: 100000, MaxMatches: 100}
}
func input() Corpus {
	return Corpus{TermsVersion: "fixture/v1", Editions: []Edition{
		{ID: "1.35", Passages: []Fields{
			{Body: []string{"enabled", "default"}, Headings: []string{"restart", "rules"}, Labels: []string{"containerrestartrules"}},
			{Body: []string{"containerrestartrules", "restart"}},
			{Body: []string{"default", "default"}},
		}},
		{ID: "1.36", Passages: []Fields{{Body: []string{"containerrestartrules", "default"}}}},
		{ID: "empty"},
	}}
}
func TestMatchAllFieldsAndEditions(t *testing.T) {
	cases := []struct {
		name, edition string
		terms         []string
		found         bool
		want          []int
	}{
		{"cross-field", "1.35", []string{"containerrestartrules", "default"}, true, []int{0}},
		{"three fields", "1.35", []string{"containerrestartrules", "default", "rules"}, true, []int{0}},
		{"label only", "1.35", []string{"containerrestartrules"}, true, []int{0, 1}},
		{"duplicates", "1.35", []string{"containerrestartrules", "containerrestartrules"}, true, []int{0, 1}},
		{"missing term", "1.35", []string{"containerrestartrules", "absent"}, true, nil},
		{"not across passages", "1.35", []string{"rules", "containerrestartrules", "restart", "only-other-passage"}, true, nil},
		{"other edition", "1.36", []string{"containerrestartrules", "default"}, true, []int{0}},
		{"known empty", "empty", []string{"default"}, true, nil},
		{"missing edition", "1.34", []string{"default"}, false, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := input()
			if tc.name == "not across passages" {
				c.Editions[0].Passages[1].Body = append(c.Editions[0].Passages[1].Body, "only-other-passage")
			}
			got, err := MatchAll(testContext(t), c, Query{TermsVersion: c.TermsVersion, Edition: tc.edition, Terms: tc.terms}, testLimits())
			want := Result{EditionFound: tc.found, Passages: tc.want}
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatal(got, err, "want", want)
			}
		})
	}
	got, err := MatchAll(testContext(t), Corpus{TermsVersion: "fixture/v1"}, Query{TermsVersion: "fixture/v1", Edition: "missing", Terms: []string{"x"}}, testLimits())
	if err != nil || !reflect.DeepEqual(got, Result{}) {
		t.Fatal(got, err)
	}
}
func TestTermsRemainExact(t *testing.T) {
	c := Corpus{TermsVersion: "exact/v1", Editions: []Edition{{ID: "Guide", Passages: []Fields{{Body: []string{"Go", "foo-bar", "é", "a b", "ContainerRestartRules"}}}}}}
	for _, term := range []string{"Go", "foo-bar", "é", "a b", "ContainerRestartRules", "go", "foo", "e\u0301", "container"} {
		got, err := MatchAll(testContext(t), c, Query{TermsVersion: c.TermsVersion, Edition: "Guide", Terms: []string{term}}, testLimits())
		expected := term == "Go" || term == "foo-bar" || term == "é" || term == "a b" || term == "ContainerRestartRules"
		if err != nil || (len(got.Passages) == 1) != expected {
			t.Fatal(term, got, err)
		}
	}
	got, err := MatchAll(testContext(t), c, Query{TermsVersion: c.TermsVersion, Edition: "guide", Terms: []string{"Go"}}, testLimits())
	if err != nil || got.EditionFound {
		t.Fatal("edition folded", got, err)
	}
}
func TestInputAndOutputOwnership(t *testing.T) {
	c := input()
	q := Query{TermsVersion: c.TermsVersion, Edition: "1.35", Terms: []string{"containerrestartrules"}}
	before, _ := json.Marshal([]any{c, q})
	first, err := MatchAll(testContext(t), c, q, testLimits())
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal([]any{c, q})
	if string(before) != string(after) {
		t.Fatal("input mutated")
	}
	first.Passages[0] = 99
	second, err := MatchAll(testContext(t), c, q, testLimits())
	if err != nil || !reflect.DeepEqual(second.Passages, []int{0, 1}) {
		t.Fatal("output aliases state", second, err)
	}
}
func TestInvalidInput(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Corpus, *Query)
	}{
		{"empty query", func(c *Corpus, q *Query) { q.Terms = nil }},
		{"empty query term", func(c *Corpus, q *Query) { q.Terms = []string{""} }},
		{"bad query term", func(c *Corpus, q *Query) { q.Terms = []string{"\xff"} }},
		{"query NUL", func(c *Corpus, q *Query) { q.Terms = []string{"x\x00"} }},
		{"empty query edition", func(c *Corpus, q *Query) { q.Edition = "" }},
		{"bad query version", func(c *Corpus, q *Query) { q.TermsVersion = "\xff" }},
		{"empty corpus version", func(c *Corpus, q *Query) { c.TermsVersion = "" }},
		{"duplicate editions", func(c *Corpus, q *Query) { c.Editions[1].ID = c.Editions[0].ID }},
		{"bad unselected edition", func(c *Corpus, q *Query) { c.Editions[1].ID = "\x00" }},
		{"bad unselected term", func(c *Corpus, q *Query) { c.Editions[1].Passages[0].Body = []string{"\xff"} }},
		{"empty body term", func(c *Corpus, q *Query) { c.Editions[0].Passages[0].Body = []string{""} }},
		{"bad heading term", func(c *Corpus, q *Query) { c.Editions[0].Passages[0].Headings = []string{"\x00"} }},
		{"bad label term", func(c *Corpus, q *Query) { c.Editions[0].Passages[0].Labels = []string{"\xff"} }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := input()
			q := Query{TermsVersion: c.TermsVersion, Edition: "1.35", Terms: []string{"default"}}
			tc.edit(&c, &q)
			result, err := MatchAll(testContext(t), c, q, testLimits())
			if !errors.Is(err, ErrInvalid) || !reflect.DeepEqual(result, Result{}) {
				t.Fatal(result, err)
			}
		})
	}
	c := input()
	result, err := MatchAll(testContext(t), c, Query{TermsVersion: "other/v1", Edition: "1.35", Terms: []string{"default"}}, testLimits())
	if !errors.Is(err, ErrIncompatible) || !reflect.DeepEqual(result, Result{}) {
		t.Fatal(result, err)
	}
}
func TestAdmissionAndResultCeilings(t *testing.T) {
	c := Corpus{TermsVersion: "v1", Editions: []Edition{{ID: "e", Passages: []Fields{{Body: []string{"x", "x"}, Headings: []string{"h"}, Labels: []string{"l"}}, {Body: []string{"x"}}}}, {ID: "other"}}}
	q := Query{TermsVersion: "v1", Edition: "e", Terms: []string{"x", "x", "h"}}
	// Versions 2+2, requested edition1, query terms3, edition IDs1+5,
	// corpus term occurrences5: all logical strings total19 bytes.
	exact := Limits{MaxEditions: 2, MaxPassages: 2, MaxTerms: 5, MaxQueryTerms: 3, MaxInputBytes: 19, MaxMatches: 2}
	result, err := MatchAll(testContext(t), c, q, exact)
	if err != nil || !reflect.DeepEqual(result.Passages, []int{0}) {
		t.Fatal(result, err)
	}
	changes := []struct {
		name string
		edit func(*Limits)
	}{
		{"editions", func(l *Limits) { l.MaxEditions-- }},
		{"passages", func(l *Limits) { l.MaxPassages-- }},
		{"corpus occurrences", func(l *Limits) { l.MaxTerms-- }},
		{"query occurrences", func(l *Limits) { l.MaxQueryTerms-- }},
		{"logical bytes", func(l *Limits) { l.MaxInputBytes-- }},
	}
	for _, tc := range changes {
		t.Run(tc.name, func(t *testing.T) {
			l := exact
			tc.edit(&l)
			result, err := MatchAll(testContext(t), c, q, l)
			if !errors.Is(err, ErrLimit) || !reflect.DeepEqual(result, Result{}) {
				t.Fatal(result, err)
			}
		})
	}
	// An unselected edition still consumes passage/term/byte admission.
	c.Editions[1].Passages = []Fields{{Body: []string{"x"}}}
	if result, err := MatchAll(testContext(t), c, q, exact); !errors.Is(err, ErrLimit) || !reflect.DeepEqual(result, Result{}) {
		t.Fatal(result, err)
	}
	c.Editions[1].Passages = nil
	q.Terms = []string{"x"}
	if result, err := MatchAll(testContext(t), c, q, exact); err != nil || !reflect.DeepEqual(result.Passages, []int{0, 1}) {
		t.Fatal("exact result ceiling", result, err)
	}
	exact.MaxMatches = 1
	if result, err := MatchAll(testContext(t), c, q, exact); !errors.Is(err, ErrLimit) || !reflect.DeepEqual(result, Result{}) {
		t.Fatal("partial output", result, err)
	}
	// Count UTF-8 bytes rather than runes.
	c = Corpus{TermsVersion: "v", Editions: []Edition{{ID: "e", Passages: []Fields{{Labels: []string{"Ω"}}}}}}
	q = Query{TermsVersion: "v", Edition: "e", Terms: []string{"Ω"}}
	l := testLimits()
	l.MaxInputBytes = 8 // v+v+e+e+Ω+Ω
	if _, err := MatchAll(testContext(t), c, q, l); err != nil {
		t.Fatal(err)
	}
	l.MaxInputBytes--
	if result, err := MatchAll(testContext(t), c, q, l); !errors.Is(err, ErrLimit) || !reflect.DeepEqual(result, Result{}) {
		t.Fatal(result, err)
	}
}
func TestPositiveLimitsAndDeadline(t *testing.T) {
	c := input()
	q := Query{TermsVersion: c.TermsVersion, Edition: "1.35", Terms: []string{"default"}}
	for field := range 6 {
		l := testLimits()
		reflect.ValueOf(&l).Elem().Field(field).SetInt(0)
		if result, err := MatchAll(testContext(t), c, q, l); !errors.Is(err, ErrInvalid) || !reflect.DeepEqual(result, Result{}) {
			t.Fatal(field, result, err)
		}
	}
	if result, err := MatchAll(context.Background(), c, q, testLimits()); !errors.Is(err, ErrInvalid) || !reflect.DeepEqual(result, Result{}) {
		t.Fatal(result, err)
	}
	canceled, cancel := context.WithCancel(testContext(t))
	cancel()
	if result, err := MatchAll(canceled, c, q, testLimits()); !errors.Is(err, context.Canceled) || !reflect.DeepEqual(result, Result{}) {
		t.Fatal(result, err)
	}
	expired, cancel := context.WithDeadline(context.Background(), time.Unix(1, 0))
	defer cancel()
	if result, err := MatchAll(expired, c, q, testLimits()); !errors.Is(err, context.DeadlineExceeded) || !reflect.DeepEqual(result, Result{}) {
		t.Fatal(result, err)
	}
}

// A deterministic context seam interrupts the final cancellation check, after
// results were built. It does not use timing or a background goroutine.
type countedContext struct {
	context.Context
	calls, stop int
}

func (c *countedContext) Err() error {
	c.calls++
	if c.stop > 0 && c.calls >= c.stop {
		return context.Canceled
	}
	return c.Context.Err()
}
func TestLateCancellationDiscardsMatches(t *testing.T) {
	c := input()
	q := Query{TermsVersion: c.TermsVersion, Edition: "1.35", Terms: []string{"containerrestartrules"}}
	count := &countedContext{Context: testContext(t)}
	if r, err := MatchAll(count, c, q, testLimits()); err != nil || len(r.Passages) != 2 {
		t.Fatal(r, err)
	}
	late := &countedContext{Context: testContext(t), stop: count.calls}
	result, err := MatchAll(late, c, q, testLimits())
	if !errors.Is(err, context.Canceled) || !reflect.DeepEqual(result, Result{}) {
		t.Fatal(result, err)
	}
}
