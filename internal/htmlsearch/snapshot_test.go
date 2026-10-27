package htmlsearch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sidsharma96/concordance/internal/htmlarticle"
	"github.com/sidsharma96/concordance/internal/htmlpassage"
	"github.com/sidsharma96/concordance/internal/htmlterms"
	"github.com/sidsharma96/concordance/internal/termsearch"
	"github.com/sidsharma96/concordance/internal/textterms"
)

func deadline(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	t.Cleanup(cancel)
	return ctx
}

func budgets() Limits {
	return Limits{
		Fields: htmlterms.Limits{
			Passages:     htmlpassage.Limits{MaxBlocks: 1000, MaxGroups: 1000, MaxInputBytes: 1 << 20, MaxPassages: 1000, MaxBodyBytes: 64 << 10, MaxReferences: 10000, MaxContextBytes: 1 << 20},
			Field:        textterms.Limits{MaxInputBytes: 64 << 10, MaxTerms: 10000, MaxTermBytes: 64 << 10, MaxOutputBytes: 128 << 10},
			MaxViewBytes: 1 << 20, MaxTerms: 100000, MaxOutputBytes: 1 << 20,
		},
		MaxEditions: 10, MaxArticles: 100, MaxBlocks: 10000, MaxGroups: 10000,
		MaxInputBytes: 1 << 20, MaxPassages: 10000, MaxTerms: 100000, MaxOutputBytes: 1 << 20,
	}
}
func searchLimits() termsearch.Limits {
	return termsearch.Limits{MaxEditions: 10, MaxPassages: 10000, MaxTerms: 100000, MaxQueryTerms: 20, MaxInputBytes: 1 << 20, MaxMatches: 10000}
}
func query(t *testing.T, edition, text string) termsearch.Query {
	t.Helper()
	p, err := textterms.Prepare(deadline(t), text, budgets().Fields.Field)
	if err != nil {
		t.Fatal(err)
	}
	return termsearch.Query{TermsVersion: p.Version, Edition: edition, Terms: p.Terms}
}
func extract(t *testing.T, content string) htmlarticle.Article {
	t.Helper()
	a, err := htmlarticle.Extract(deadline(t), []byte(`<article class="doc">`+content+`</article>`),
		htmlarticle.Profile{ID: "fixture/v1", ArticleClass: "doc", NoticeClasses: []string{"notice"}},
		htmlarticle.Limits{MaxInputBytes: 1 << 20, MaxTokenBytes: 64 << 10, MaxTokens: 10000, MaxNodes: 10000, MaxDepth: 100, MaxBlocks: 1000, MaxGroups: 1000, MaxBlockBytes: 64 << 10, MaxOutputBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	return a
}
func smallArticle() htmlarticle.Article {
	return htmlarticle.Article{Processor: htmlarticle.Processor, Profile: "fixture/v1",
		Blocks: []htmlarticle.Block{{Kind: htmlarticle.Prose, Text: "Alpha"}, {Kind: htmlarticle.Prose, Text: "Beta"}},
		Groups: []htmlarticle.Group{{Kind: htmlarticle.Notice, First: 0, After: 2, Label: "Feature"}},
	}
}
func fixture(t *testing.T) []Edition {
	return []Edition{
		{ID: "1.35", Articles: []htmlarticle.Article{
			extract(t, `<h1>Pod</h1><h2>Restart policy</h2><p>Introduction.</p>
<div class="notice" title="ContainerRestartRules"><p>Enabled by default.</p></div>
<pre>  restartPolicy: Never
</pre><figure><img src="https://onlyurl.invalid/figure" alt="Original diagram"><figcaption>Authored caption</figcaption></figure>`),
			extract(t, `<h1>Another</h1><p>Default policy explanation.</p><p>Other answer.</p>`),
		}},
		{ID: "1.34", Articles: []htmlarticle.Article{extract(t, `<h1>Pod</h1><p>Other version.</p>`)}},
		{ID: "empty"},
	}
}
func mustNew(t *testing.T, input []Edition) *Snapshot {
	t.Helper()
	s, err := New(deadline(t), input, budgets())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestExtractSearchLocationQuote(t *testing.T) {
	input := fixture(t)
	before, _ := json.Marshal(input)
	s := mustNew(t, input)
	for _, text := range []string{"default", "ContainerRestartRules default", "restartPolicy Never", "diagram caption", "other", "absent"} {
		checkComposition(t, input, s, query(t, "1.35", text))
	}
	for _, tc := range []struct{ text, want string }{
		{"ContainerRestartRules default", "Enabled by default."},
		{"restartPolicy Never", "  restartPolicy: Never\n"},
		{"diagram caption", "Original diagram\nAuthored caption"},
	} {
		r, err := s.Search(deadline(t), query(t, "1.35", tc.text), searchLimits())
		if err != nil || len(r.Hits) != 1 {
			t.Fatal(tc, r, err)
		}
		quote, err := s.Quote(deadline(t), r.Hits[0].Ref, 10000)
		if err != nil || quote != tc.want {
			t.Fatalf("%s: %q %v", tc.text, quote, err)
		}
	}
	after, _ := json.Marshal(input)
	if string(before) != string(after) {
		t.Fatal("construction/search/quote mutated input")
	}
}

func TestReferencesAndOwnedResults(t *testing.T) {
	input := []Edition{{ID: "chosen", Articles: []htmlarticle.Article{smallArticle()}}}
	s := mustNew(t, input)
	r, err := s.Search(deadline(t), query(t, "chosen", "feature"), searchLimits())
	if err != nil || len(r.Hits) != 1 {
		t.Fatal(r, err)
	}
	ref := r.Hits[0].Ref
	want := Location{Edition: 0, Article: 0, Passage: 0, Body: htmlpassage.Range{First: 0, After: 2}}
	loc, err := s.Location(ref)
	if err != nil || loc != want {
		t.Fatal(loc, err)
	}
	loc.Body.First = 99
	if again, err := s.Location(ref); err != nil || again != want {
		t.Fatal("location values alias registry", again, err)
	}
	r.Hits[0] = Hit{Score: -1}
	again, err := s.Search(deadline(t), query(t, "chosen", "feature"), searchLimits())
	if err != nil || len(again.Hits) != 1 || again.Hits[0].Ref != ref || again.Hits[0].Score <= 0 {
		t.Fatal("result storage aliases snapshot", again, err)
	}
	copy := *s // Same state, not another construction.
	if text, err := copy.Quote(deadline(t), ref, 10); err != nil || text != "Alpha\nBeta" {
		t.Fatal(text, err)
	}
	independent := mustNew(t, input)
	for _, invalid := range []Reference{{}, independentRef(t, independent)} {
		if loc, err := s.Location(invalid); !errors.Is(err, ErrInvalid) || loc != (Location{}) {
			t.Fatal(loc, err)
		}
		if text, err := s.Quote(deadline(t), invalid, 100); !errors.Is(err, ErrInvalid) || text != "" {
			t.Fatal(text, err)
		}
	}
}
func independentRef(t *testing.T, s *Snapshot) Reference {
	t.Helper()
	r, err := s.Search(deadline(t), query(t, "chosen", "feature"), searchLimits())
	if err != nil || len(r.Hits) != 1 {
		t.Fatal(r, err)
	}
	return r.Hits[0].Ref
}

func TestQuoteLimitsAndEmptyText(t *testing.T) {
	s := mustNew(t, []Edition{{ID: "chosen", Articles: []htmlarticle.Article{smallArticle()}}})
	ref := independentRef(t, s)
	if text, err := s.Quote(deadline(t), ref, 10); err != nil || text != "Alpha\nBeta" {
		t.Fatal(text, err)
	}
	for _, n := range []int{1, 5, 9} {
		if text, err := s.Quote(deadline(t), ref, n); !errors.Is(err, ErrLimit) || text != "" {
			t.Fatal(n, text, err)
		}
	}
	for _, n := range []int{0, -1} {
		if text, err := s.Quote(deadline(t), ref, n); !errors.Is(err, ErrInvalid) || text != "" {
			t.Fatal(n, text, err)
		}
	}
	if text, err := s.Quote(context.Background(), ref, 10); !errors.Is(err, ErrInvalid) || text != "" {
		t.Fatal(text, err)
	}
	// A valid nonempty body range can contain no text but still match its label.
	a := smallArticle()
	a.Blocks = a.Blocks[:1]
	a.Blocks[0].Text = ""
	a.Groups[0].After = 1
	empty := mustNew(t, []Edition{{ID: "chosen", Articles: []htmlarticle.Article{a}}})
	if text, err := empty.Quote(deadline(t), independentRef(t, empty), 1); err != nil || text != "" {
		t.Fatal("empty quote is not a failure", text, err)
	}
}

func TestMissingEmptyAndSearchFailures(t *testing.T) {
	s := mustNew(t, []Edition{{ID: "chosen", Articles: []htmlarticle.Article{smallArticle(), smallArticle()}}, {ID: "empty"}})
	for _, edition := range []string{"empty", "missing", "CHOSEN"} {
		got, err := s.Search(deadline(t), query(t, edition, "alpha"), searchLimits())
		want := Result{EditionFound: edition == "empty", RankingVersion: termsearch.BM25Version}
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatal(got, err, want)
		}
	}
	noEditions := mustNew(t, nil)
	if got, err := noEditions.Search(deadline(t), query(t, "missing", "alpha"), searchLimits()); err != nil || got.EditionFound || len(got.Hits) != 0 || got.RankingVersion != termsearch.BM25Version {
		t.Fatal(got, err)
	}
	for _, kind := range []string{"excess matches", "term version", "empty query", "no deadline"} {
		q, l, ctx := query(t, "chosen", "alpha"), searchLimits(), deadline(t)
		var want error
		switch kind {
		case "excess matches":
			l.MaxMatches, want = 1, termsearch.ErrLimit
		case "term version":
			q.TermsVersion, want = "incompatible", termsearch.ErrIncompatible
		case "empty query":
			q.Terms, want = nil, termsearch.ErrInvalid
		case "no deadline":
			ctx, want = context.Background(), termsearch.ErrInvalid
		}
		if got, err := s.Search(ctx, q, l); !errors.Is(err, want) || !reflect.DeepEqual(got, Result{}) {
			t.Fatal(kind, got, err)
		}
	}
	for _, invalid := range []*Snapshot{nil, {}} {
		if loc, err := invalid.Location(Reference{}); !errors.Is(err, ErrInvalid) || loc != (Location{}) {
			t.Fatal(loc, err)
		}
		if text, err := invalid.Quote(deadline(t), Reference{}, 10); !errors.Is(err, ErrInvalid) || text != "" {
			t.Fatal(text, err)
		}
		if got, err := invalid.Search(deadline(t), query(t, "chosen", "alpha"), searchLimits()); !errors.Is(err, ErrInvalid) || !reflect.DeepEqual(got, Result{}) {
			t.Fatal(got, err)
		}
	}
}

func TestConcurrentReaders(t *testing.T) {
	input := fixture(t)
	s := mustNew(t, input)
	q, ctx := query(t, "1.35", "default"), deadline(t)
	want, err := s.Search(ctx, q, searchLimits())
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	failures := make(chan error, 6)
	for range 6 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 10 {
				got, err := s.Search(ctx, q, searchLimits())
				if err != nil || !reflect.DeepEqual(got, want) {
					failures <- fmt.Errorf("reader disagrees: %v", err)
					return
				}
				for _, hit := range got.Hits {
					loc, err := s.Location(hit.Ref)
					if err != nil {
						failures <- err
						return
					}
					text, err := s.Quote(ctx, hit.Ref, 10000)
					if err != nil || text != oracleQuote(input, loc) {
						failures <- fmt.Errorf("quote disagrees: %v", err)
						return
					}
				}
			}
		}()
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		t.Error(err)
	}
}

func oracleQuote(input []Edition, loc Location) string {
	var parts []string
	for _, b := range input[loc.Edition].Articles[loc.Article].Blocks[loc.Body.First:loc.Body.After] {
		parts = append(parts, b.Text)
	}
	return strings.Join(parts, "\n")
}
