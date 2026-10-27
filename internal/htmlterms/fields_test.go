package htmlterms

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/sidsharma96/concordance/internal/htmlarticle"
	"github.com/sidsharma96/concordance/internal/htmlpassage"
	"github.com/sidsharma96/concordance/internal/termsearch"
	"github.com/sidsharma96/concordance/internal/textterms"
)

func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func testLimits() Limits {
	return Limits{
		Passages:     htmlpassage.Limits{MaxBlocks: 1000, MaxGroups: 1000, MaxInputBytes: 1 << 20, MaxPassages: 1000, MaxBodyBytes: 64 << 10, MaxReferences: 10000, MaxContextBytes: 1 << 20},
		Field:        textterms.Limits{MaxInputBytes: 64 << 10, MaxTerms: 10000, MaxTermBytes: 64 << 10, MaxOutputBytes: 128 << 10},
		MaxViewBytes: 1 << 20, MaxTerms: 100000, MaxOutputBytes: 1 << 20,
	}
}

func extract(t *testing.T, content string) htmlarticle.Article {
	t.Helper()
	a, err := htmlarticle.Extract(testContext(t), []byte(`<article class="doc">`+content+`</article>`),
		htmlarticle.Profile{ID: "fixture/v1", ArticleClass: "doc", NoticeClasses: []string{"notice"}},
		htmlarticle.Limits{MaxInputBytes: 1 << 20, MaxTokenBytes: 1 << 16, MaxTokens: 10000, MaxNodes: 10000, MaxDepth: 100, MaxBlocks: 1000, MaxGroups: 1000, MaxBlockBytes: 64 << 10, MaxOutputBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func has(terms []string, wanted string) bool {
	for _, term := range terms {
		if term == wanted {
			return true
		}
	}
	return false
}

func TestAncestorFieldsAndExactMatching(t *testing.T) {
	a := extract(t, `<h1>Pod</h1><h2>Restart policy</h2><p>Introduction.</p>
<h3>Behavior</h3><p>The default is Always.</p>
<div class="notice" title="ContainerRestartRules"><p>Enabled by default.</p></div>
<h2>Unrelated</h2><p>Sibling.</p><pre>readiness-probe v1.35</pre>
<figure><img src="https://onlyurlmarker.invalid/picture" alt="Original diagram"><figcaption>Authored caption</figcaption></figure>`)
	before, _ := json.Marshal(a)
	got, err := Build(testContext(t), a, testLimits())
	if err != nil || got.Processor != Processor || got.TermsVersion != textterms.Version || got.Plan.Processor != htmlpassage.Processor || len(got.Fields) != len(got.Plan.Passages) {
		t.Fatal(got, err)
	}
	after, _ := json.Marshal(a)
	if string(before) != string(after) {
		t.Fatal("article mutated")
	}
	answer, notice, sibling, code, figure := -1, -1, -1, -1, -1
	for i, f := range got.Fields {
		if has(f.Body, "always") {
			answer = i
			if !has(f.Headings, "pod") || !has(f.Headings, "restart") || has(f.Headings, "behavior") || has(f.Headings, "unrelated") {
				t.Fatal("ancestor scope or own-heading duplication", f)
			}
		}
		if has(f.Labels, "containerrestartrules") {
			notice = i
			if has(f.Body, "containerrestartrules") {
				t.Fatal("metadata entered body quotation")
			}
		}
		if has(f.Body, "sibling") {
			sibling = i
			if has(f.Headings, "restart") || !has(f.Headings, "pod") {
				t.Fatal("sibling heading leaked", f)
			}
		}
		if has(f.Body, "readiness-probe") {
			code = i
		}
		if has(f.Body, "diagram") {
			figure = i
			if !has(f.Body, "caption") {
				t.Fatal("caption omitted")
			}
		}
		for _, field := range [3][]string{f.Body, f.Headings, f.Labels} {
			for _, term := range field {
				if strings.Contains(term, "onlyurlmarker") {
					t.Fatal("image URL entered searchable text")
				}
			}
		}
	}
	if min(answer, notice, sibling, code, figure) < 0 {
		t.Fatal("fixture coverage missing", answer, notice, sibling, code, figure)
	}
	corpus := termsearch.Corpus{TermsVersion: got.TermsVersion, Editions: []termsearch.Edition{{ID: "1.35", Passages: got.Fields}, {ID: "1.36"}}}
	for _, tc := range []struct {
		query, edition string
		want           []int
	}{
		{"pod restart default always", "1.35", []int{answer}},
		{"ContainerRestartRules default", "1.35", []int{notice}},
		{"readiness-probe v1.35", "1.35", []int{code}},
		{"restart sibling", "1.35", nil},
		{"always caption", "1.35", nil},
		{"pod restart default always", "1.36", nil},
	} {
		q, err := textterms.Prepare(testContext(t), tc.query, testLimits().Field)
		if err != nil {
			t.Fatal(err)
		}
		result, err := termsearch.MatchAll(testContext(t), corpus, termsearch.Query{TermsVersion: q.Version, Edition: tc.edition, Terms: q.Terms},
			termsearch.Limits{MaxEditions: 2, MaxPassages: 1000, MaxTerms: 100000, MaxQueryTerms: 20, MaxInputBytes: 1 << 20, MaxMatches: 1000})
		if err != nil || !result.EditionFound || !reflect.DeepEqual(result.Passages, tc.want) {
			t.Fatal(tc, result, err)
		}
	}
}

func smallArticle() htmlarticle.Article {
	return htmlarticle.Article{Processor: htmlarticle.Processor, Profile: "fixture/v1",
		Blocks: []htmlarticle.Block{{Kind: htmlarticle.Prose, Text: "a"}, {Kind: htmlarticle.Prose, Text: "b"}},
		Groups: []htmlarticle.Group{{Kind: htmlarticle.Notice, First: 0, After: 2, Label: "tag"}},
	}
}

func TestBoundariesAndOccurrences(t *testing.T) {
	a := smallArticle()
	got, err := Build(testContext(t), a, testLimits())
	if err != nil || len(got.Fields) != 1 || !reflect.DeepEqual(got.Fields[0], termsearch.Fields{Body: []string{"a", "b"}, Labels: []string{"tag"}}) {
		t.Fatal("blocks concatenated or labels misplaced", got, err)
	}
	a.Blocks[1].Text = "a"
	got, err = Build(testContext(t), a, testLimits())
	if err != nil || !reflect.DeepEqual(got.Fields[0].Body, []string{"a", "a"}) {
		t.Fatal("authored occurrences deduplicated", got, err)
	}
}

func TestExactAndExcessiveLimits(t *testing.T) {
	base := testLimits()
	base.MaxViewBytes, base.MaxTerms, base.MaxOutputBytes = 6, 3, 5
	base.Field = textterms.Limits{MaxInputBytes: 3, MaxTerms: 2, MaxTermBytes: 3, MaxOutputBytes: 3}
	if _, err := Build(testContext(t), smallArticle(), base); err != nil {
		t.Fatal("exact budget failed", err)
	}
	for _, tc := range []struct {
		name string
		edit func(*Limits)
		want error
	}{
		{"aggregate views incl LF", func(l *Limits) { l.MaxViewBytes-- }, ErrLimit},
		{"aggregate later-field terms", func(l *Limits) { l.MaxTerms-- }, ErrLimit},
		{"aggregate later-field bytes", func(l *Limits) { l.MaxOutputBytes-- }, ErrLimit},
		{"field raw bytes incl LF", func(l *Limits) { l.Field.MaxInputBytes-- }, ErrLimit},
		{"field terms", func(l *Limits) { l.Field.MaxTerms-- }, textterms.ErrLimit},
		{"later-field term bytes", func(l *Limits) { l.Field.MaxTermBytes-- }, textterms.ErrLimit},
		{"later-field output bytes", func(l *Limits) { l.Field.MaxOutputBytes-- }, textterms.ErrLimit},
		{"passage body bytes", func(l *Limits) { l.Passages.MaxBodyBytes = 2 }, htmlpassage.ErrLimit},
		{"invalid field limit", func(l *Limits) { l.Field.MaxTerms = 0 }, ErrInvalid},
		{"invalid aggregate", func(l *Limits) { l.MaxTerms = 0 }, ErrInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			limits := base
			tc.edit(&limits)
			got, err := Build(testContext(t), smallArticle(), limits)
			if !errors.Is(err, tc.want) || !reflect.DeepEqual(got, Prepared{}) {
				t.Fatal("partial result or wrong error", got, err)
			}
		})
	}
}

func TestMalformedArticleAndIndependentOutput(t *testing.T) {
	for _, a := range []htmlarticle.Article{
		{},
		{Processor: htmlarticle.Processor, Profile: "fixture/v1", Blocks: []htmlarticle.Block{{Kind: htmlarticle.Prose, Text: "bad\x00"}}},
		{Processor: "unsupported", Profile: "fixture/v1", Blocks: []htmlarticle.Block{{Kind: htmlarticle.Prose, Text: "x"}}},
	} {
		got, err := Build(testContext(t), a, testLimits())
		if err == nil || !reflect.DeepEqual(got, Prepared{}) {
			t.Fatal(got, err)
		}
	}
	a := smallArticle()
	one, err := Build(testContext(t), a, testLimits())
	if err != nil {
		t.Fatal(err)
	}
	one.Fields[0].Body[0] = "changed"
	one.Plan.Passages[0].Body.First = 99
	two, err := Build(testContext(t), a, testLimits())
	if err != nil || two.Fields[0].Body[0] != "a" || two.Plan.Passages[0].Body.First != 0 || a.Blocks[0].Text != "a" {
		t.Fatal("aliased result or input", two, err)
	}
}

type countedContext struct {
	context.Context
	checks, cancelAt int
}

func (c *countedContext) Err() error {
	c.checks++
	if c.cancelAt > 0 && c.checks >= c.cancelAt {
		return context.Canceled
	}
	return c.Context.Err()
}

func TestCancellationAndDeadline(t *testing.T) {
	parent := testContext(t)
	canceled, cancel := context.WithCancel(parent)
	cancel()
	for _, tc := range []struct {
		ctx  context.Context
		want error
	}{{t.Context(), ErrInvalid}, {canceled, context.Canceled}} {
		got, err := Build(tc.ctx, smallArticle(), testLimits())
		if !errors.Is(err, tc.want) || !reflect.DeepEqual(got, Prepared{}) {
			t.Fatal(got, err)
		}
	}
	count := &countedContext{Context: parent}
	if _, err := Build(count, smallArticle(), testLimits()); err != nil {
		t.Fatal(err)
	}
	for _, at := range []int{1, count.checks / 2, count.checks} {
		got, err := Build(&countedContext{Context: parent, cancelAt: at}, smallArticle(), testLimits())
		if !errors.Is(err, context.Canceled) || !reflect.DeepEqual(got, Prepared{}) {
			t.Fatal("partial cancellation result", at, got, err)
		}
	}
}
