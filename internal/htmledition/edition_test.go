package htmledition

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sidsharma96/concordance/internal/htmlarticle"
	"github.com/sidsharma96/concordance/internal/htmlarticlejson"
	"github.com/sidsharma96/concordance/internal/htmlpassage"
	"github.com/sidsharma96/concordance/internal/htmlsearch"
	"github.com/sidsharma96/concordance/internal/htmlterms"
	"github.com/sidsharma96/concordance/internal/termsearch"
	"github.com/sidsharma96/concordance/internal/textterms"
)

func deadline(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	t.Cleanup(cancel)
	return ctx
}
func limits() Limits {
	return Limits{MaxEncodedBytes: 1 << 20, MaxArticles: 100, MaxArticleBytes: 2 << 20, MaxBlocks: 10000, MaxPassages: 1000, MaxTerms: 10000, MaxPreparedBytes: 1 << 20, MaxKeyBytes: 1 << 20, MaxTermBytes: 10000, MaxVocabulary: 10000, MaxPostings: 10000}
}
func articleLimits() htmlarticlejson.Limits {
	return htmlarticlejson.Limits{MaxEncodedBytes: 1 << 20, MaxBlocks: 1000, MaxGroups: 1000, MaxStringBytes: 10000, MaxValueBytes: 1 << 20, MaxGroupDepth: 100, MaxHTMLBytes: 1 << 20}
}
func preparationLimits() htmlterms.Limits {
	return htmlterms.Limits{Passages: htmlpassage.Limits{MaxBlocks: 1000, MaxGroups: 1000, MaxInputBytes: 1 << 20, MaxPassages: 1000, MaxBodyBytes: 10000, MaxReferences: 10000, MaxContextBytes: 1 << 20}, Field: textterms.Limits{MaxInputBytes: 10000, MaxTerms: 10000, MaxTermBytes: 10000, MaxOutputBytes: 1 << 20}, MaxViewBytes: 1 << 20, MaxTerms: 10000, MaxOutputBytes: 1 << 20}
}
func buildLimits() BuildLimits { return BuildLimits{limits(), articleLimits(), preparationLimits()} }
func searchLimits() termsearch.Limits {
	return termsearch.Limits{MaxEditions: 1, MaxPassages: 1000, MaxTerms: 10000, MaxQueryTerms: 10, MaxInputBytes: 1 << 20, MaxMatches: 1000}
}
func reference(body []byte) Ref { return Ref{sha256.Sum256(body), len(body)} }
func fixture(t *testing.T) (context.Context, [][]byte, []htmlarticle.Article, *Edition, []byte) {
	t.Helper()
	ctx := deadline(t)
	var articles []htmlarticle.Article
	var bodies [][]byte
	for _, html := range []string{`<h1>Pod</h1><div class="notice" title="ContainerRestartRules"><p>Enabled by default.</p><pre>restartPolicy: Never</pre></div><p>中文 😀 �</p>`, `<h1>Pod</h1><p>restartPolicy: Never</p><p>Enabled by default.</p>`} {
		a, err := htmlarticle.Extract(ctx, []byte(`<article class="doc">`+html+`</article>`), htmlarticle.Profile{ID: "fixture/v1", ArticleClass: "doc", NoticeClasses: []string{"notice"}}, htmlarticle.Limits{MaxInputBytes: 10000, MaxTokenBytes: 10000, MaxTokens: 1000, MaxNodes: 1000, MaxDepth: 100, MaxBlocks: 1000, MaxGroups: 1000, MaxBlockBytes: 10000, MaxOutputBytes: 100000})
		if err != nil {
			t.Fatal(err)
		}
		b, err := htmlarticlejson.Encode(ctx, a, articleLimits())
		if err != nil {
			t.Fatal(err)
		}
		articles = append(articles, a)
		bodies = append(bodies, b)
	}
	e, err := Build(ctx, "one", bodies, buildLimits())
	if err != nil {
		t.Fatal(err)
	}
	saved, err := Encode(ctx, e, limits())
	if err != nil {
		t.Fatal(err)
	}
	return ctx, bodies, articles, e, saved
}
func TestReopenMatchesSnapshotQuotesAndOwnership(t *testing.T) {
	ctx, bodies, articles, built, saved := fixture(t)
	before := bytes.Clone(saved)
	e, err := Open(ctx, saved, reference(saved), "one", limits())
	if err != nil {
		t.Fatal(err)
	}
	clear(saved)
	again, err := Encode(ctx, e, limits())
	if err != nil || !bytes.Equal(before, again) {
		t.Fatal("canonical reopen", err)
	}
	snapshot, err := htmlsearch.New(ctx, []htmlsearch.Edition{{ID: "one", Articles: articles}}, htmlsearch.Limits{Fields: preparationLimits(), MaxEditions: 1, MaxArticles: 100, MaxBlocks: 10000, MaxGroups: 10000, MaxInputBytes: 2 << 20, MaxPassages: 1000, MaxTerms: 10000, MaxOutputBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"Pod", "ContainerRestartRules default", "restartPolicy Never", "中文", "absent"} {
		prepared, err := textterms.Prepare(ctx, text, preparationLimits().Field)
		if err != nil {
			t.Fatal(err)
		}
		q := termsearch.Query{TermsVersion: prepared.Version, Edition: "one", Terms: prepared.Terms}
		want, err := snapshot.Search(ctx, q, searchLimits())
		if err != nil {
			t.Fatal(err)
		}
		got, err := e.Search(ctx, q, searchLimits())
		if err != nil || len(got.Matches) != len(want.Hits) {
			t.Fatal(got, want, err)
		}
		for i, hit := range got.Matches {
			loc, err := e.Location(hit.Passage)
			if err != nil {
				t.Fatal(err)
			}
			expected, err := snapshot.Location(want.Hits[i].Ref)
			if err != nil || loc.Article != expected.Article || loc.Body != expected.Body || hit.Score != want.Hits[i].Score {
				t.Fatal("location/score", err)
			}
			quote, err := e.Quote(ctx, hit.Passage, bodies[loc.Article], articleLimits(), 10000)
			if err != nil {
				t.Fatal(err)
			}
			original, err := snapshot.Quote(ctx, want.Hits[i].Ref, 10000)
			if err != nil || quote != original {
				t.Fatal(quote, original, err)
			}
			if len(quote) > 0 {
				if short, err := e.Quote(ctx, hit.Passage, bodies[loc.Article], articleLimits(), len(quote)-1); err == nil || short != "" {
					t.Fatal("quote bound", err)
				}
			}
		}
	}
	view, err := Inspect(ctx, e, limits(), 1<<20)
	if err != nil || !json.Valid(view) {
		t.Fatal("inspect", err)
	}
	var inspected struct {
		Format string
		Words  []struct {
			Term string
			Rows [][4]int
		}
		Articles []any
		Passages []any
	}
	if err := json.Unmarshal(view, &inspected); err != nil || inspected.Format != Format || len(inspected.Articles) != 2 || len(inspected.Words) == 0 {
		t.Fatal(string(view), err)
	}
	if out, err := Inspect(ctx, e, limits(), len(view)-1); !errors.Is(err, ErrLimit) || out != nil {
		t.Fatal("inspection bound", err)
	}
	clear(view)
	for _, b := range bodies {
		clear(b)
	}
	if rebuilt, err := Encode(ctx, built, limits()); err != nil || !bytes.Equal(before, rebuilt) {
		t.Fatal("builder retained article aliases", err)
	}
}
func TestKnownEmptyAndZeroTermPassage(t *testing.T) {
	ctx := deadline(t)
	e, err := Build(ctx, "empty", nil, buildLimits())
	if err != nil {
		t.Fatal(err)
	}
	body, err := Encode(ctx, e, limits())
	if err != nil {
		t.Fatal(err)
	}
	golden := "CNDX\x00\x01\x05empty\x16concordance.article/v1\x0ehtmlpassage/v1\x0chtmlterms/v1\x1btechnical/v1/unicode-17.0.0\x00\x00\x00\x00\x00"
	if string(body) != golden {
		t.Fatalf("wire changed: %x", body)
	}
	loaded, err := Open(ctx, body, reference(body), "empty", limits())
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"empty", "missing"} {
		got, err := loaded.Search(ctx, termsearch.Query{TermsVersion: termsVersion, Edition: id, Terms: []string{"word"}}, searchLimits())
		if err != nil || got.EditionFound != (id == "empty") || len(got.Matches) != 0 {
			t.Fatal(got, err)
		}
	}
	a := htmlarticle.Article{Processor: "htmlarticle/v1", Profile: "zero/v1", HTMLBytes: 1, Blocks: []htmlarticle.Block{{Kind: htmlarticle.Prose, Text: "😀"}}}
	raw, err := htmlarticlejson.Encode(ctx, a, articleLimits())
	if err != nil {
		t.Fatal(err)
	}
	zero, err := Build(ctx, "zero", [][]byte{raw, raw}, buildLimits())
	if err != nil || len(zero.articles) != 2 || zero.articles[0].Object != zero.articles[1].Object {
		t.Fatal("duplicate bytes slots", err)
	}
	encoded, err := Encode(ctx, zero, limits())
	if err != nil {
		t.Fatal(err)
	}
	opened, err := Open(ctx, encoded, reference(encoded), "zero", limits())
	if err != nil {
		t.Fatal(err)
	}
	quote, err := opened.Quote(ctx, 1, raw, articleLimits(), 10)
	if err != nil || quote != "😀" {
		t.Fatal(quote, err)
	}
}
func TestBindingErrorsConcurrencyAndInvalidState(t *testing.T) {
	ctx, bodies, _, _, saved := fixture(t)
	expected := reference(saved)
	if e, err := Open(ctx, saved, expected, "wrong", limits()); !errors.Is(err, ErrMismatch) || e != nil {
		t.Fatal(e, err)
	}
	corrupt := bytes.Clone(saved)
	corrupt[len(corrupt)-1] ^= 1
	if e, err := Open(ctx, corrupt, expected, "one", limits()); !errors.Is(err, ErrMismatch) || e != nil {
		t.Fatal(e, err)
	}
	expected.Bytes++
	if e, err := Open(ctx, saved, expected, "one", limits()); !errors.Is(err, ErrMismatch) || e != nil {
		t.Fatal(e, err)
	}
	e, err := Open(ctx, saved, reference(saved), "one", limits())
	if err != nil {
		t.Fatal(err)
	}
	if q, err := e.Quote(ctx, 0, bodies[1], articleLimits(), 10000); !errors.Is(err, ErrMismatch) || q != "" {
		t.Fatal(q, err)
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 5 {
				b, err := Encode(ctx, e, limits())
				if err != nil || !bytes.Equal(b, saved) {
					t.Error("concurrent encode", err)
				}
				q := termsearch.Query{TermsVersion: termsVersion, Edition: "one", Terms: []string{"pod"}}
				if _, err := e.Search(ctx, q, searchLimits()); err != nil {
					t.Error(err)
				}
				if _, err := e.Quote(ctx, 0, bodies[0], articleLimits(), 10000); err != nil {
					t.Error(err)
				}
			}
		})
	}
	wg.Wait()
	for _, bad := range []context.Context{nil, context.Background()} {
		if out, err := Open(bad, saved, reference(saved), "one", limits()); err == nil || out != nil {
			t.Fatal(err)
		}
		if out, err := Encode(bad, e, limits()); err == nil || out != nil {
			t.Fatal(err)
		}
	}
	var invalid *Edition
	if _, err := invalid.Location(0); err == nil {
		t.Fatal("zero location")
	}
	if _, err := invalid.Article(0); err == nil {
		t.Fatal("zero article")
	}
	if invalid.ID() != "" {
		t.Fatal("zero ID")
	}
	if _, err := e.Location(-1); err == nil {
		t.Fatal("negative position")
	}
	if _, err := Build(ctx, "one", nil, BuildLimits{}); err == nil {
		t.Fatal("invalid configuration")
	}
	if _, err := Build(ctx, strings.Repeat("e", 100), nil, BuildLimits{File: Limits{}, Article: articleLimits(), Preparation: preparationLimits()}); err == nil {
		t.Fatal("invalid limits")
	}
	// Inspect returns values rather than exposing mutable descriptor/slice aliases.
	a, _ := e.Article(0)
	a.Blocks = 1
	if reflect.DeepEqual(a, e.articles[0]) {
		t.Fatal("descriptor mutation")
	}
}
