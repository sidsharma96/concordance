package publishedsearch

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/sidsharma96/concordance/internal/catalog"
	gm "github.com/sidsharma96/concordance/internal/generationmanifest"
	"github.com/sidsharma96/concordance/internal/htmledition"
	"github.com/sidsharma96/concordance/internal/objectstore"
	"github.com/sidsharma96/concordance/internal/termsearch"
)

func TestReaderReusesOpenedStateAndPinsGeneration(t *testing.T) {
	f := setup(t)
	a := makeBundle(t, "generation-a", "Retry uploads safely.")
	f.publish(t, a)
	f.remote.reset()
	r := f.open(t, "generation-a")
	q := prepared(t, "retry")
	oracle, err := a.index.Search(testContext(t), q, queryLimits())
	if err != nil {
		t.Fatal(err)
	}
	var first Result
	for range 100 {
		got, err := r.Search(testContext(t), q, queryLimits())
		if err != nil || len(got.Hits) != len(oracle.Matches) || len(got.Hits) != 2 {
			t.Fatal("search", got, err)
		}
		for i, h := range got.Hits {
			if h.Score != oracle.Matches[i].Score || h.Ref.passage != oracle.Matches[i].Passage {
				t.Fatal("rank changed")
			}
		}
		if first.Hits == nil {
			first = got
		}
	}
	if got := f.remote.counts(); !reflect.DeepEqual(got, map[string]int{"GET": 2}) {
		t.Fatal("search fetched remote bodies", got)
	}
	t.Log("100 searches: 2 opening GETs, no search I/O; exact ranked scores/order preserved")
	f.remote.reset()
	for range 100 {
		fresh := f.open(t, "generation-a")
		got, err := fresh.Search(testContext(t), q, queryLimits())
		if err != nil || len(got.Hits) != 2 || got.Hits[0].Score != first.Hits[0].Score {
			t.Fatal("one-shot comparison changed results", err)
		}
	}
	if !reflect.DeepEqual(f.remote.counts(), map[string]int{"GET": 200}) {
		t.Fatal("one-shot comparison request count", f.remote.counts())
	}
	t.Log("100 separate opens/searches: 200 GETs; this counts requests, not latency")
	left, err := r.Location(testContext(t), first.Hits[0].Ref)
	if err != nil {
		t.Fatal(err)
	}
	right, err := r.Location(testContext(t), first.Hits[1].Ref)
	if err != nil || left.Document.NativeID == right.Document.NativeID || left.Document.Article != right.Document.Article {
		t.Fatal("lost shared-byte document identity", err)
	}
	left.Selection.Generation = "edited"
	left.Document.Images[0].Address = "/edited.svg"
	again, err := r.Location(testContext(t), first.Hits[0].Ref)
	if err != nil || again.Selection.Generation != "generation-a" || again.Document.Images[0].Address == "/edited.svg" {
		t.Fatal("mutable metadata escaped", err)
	}

	b := makeBundle(t, "generation-b", "Inspect failures first.")
	f.publish(t, b)
	rb := f.open(t, "generation-b")
	f.remote.reset()
	after, err := r.Search(testContext(t), q, queryLimits())
	if err != nil || !reflect.DeepEqual(after, first) {
		t.Fatal("new publication changed old reader", err)
	}
	newResult, err := rb.Search(testContext(t), q, queryLimits())
	if err != nil || len(newResult.Hits) != 0 {
		t.Fatal("new reader did not select new bytes", err)
	}
	if len(f.remote.counts()) != 0 {
		t.Fatal("local searches contacted storage")
	}
	want, err := a.index.Quote(testContext(t), oracle.Matches[0].Passage, a.article, testLimits().Article, 4096)
	if err != nil {
		t.Fatal(err)
	}
	clear(a.article)
	clear(a.manifest)
	for _, body := range a.artifacts {
		clear(body)
	}
	quote, err := r.Quote(testContext(t), first.Hits[0].Ref, 4096)
	if err != nil || quote != want || !strings.Contains(quote, "Retry uploads safely.") {
		t.Fatal("old quote changed", quote, err)
	}
	if !reflect.DeepEqual(f.remote.counts(), map[string]int{"GET": 1}) {
		t.Fatal("quote fetched more than one body")
	}
	f.remote.reset()
	if got, err := rb.Quote(testContext(t), first.Hits[0].Ref, 4096); !errors.Is(err, ErrInvalid) || got != "" {
		t.Fatal("cross-reader handle accepted", err)
	}
	if len(f.remote.counts()) != 0 {
		t.Fatal("foreign handle caused I/O")
	}
	copyReader := *r
	if got, err := copyReader.Quote(testContext(t), first.Hits[0].Ref, 4096); err != nil || got != want {
		t.Fatal("wrapper copy lost state association", err)
	}
	t.Log("publishing B preserves A's searches and exact quotes; B rejects A's handles before I/O")
}

func TestReaderStrictReadsSurviveQuarantineAndCatalogClosure(t *testing.T) {
	f := setup(t)
	b := makeBundle(t, "generation-a", "Retry uploads safely.")
	f.publish(t, b)
	var receipts int
	if err := f.pool.QueryRow(testContext(t), "SELECT count(*) FROM concordance_object_receipts").Scan(&receipts); err != nil || receipts == 0 {
		t.Fatal("fixture has no verification receipts", err)
	}
	if err := f.cat.Quarantine(testContext(t), f.namespace); err != nil {
		t.Fatal(err)
	}
	f.remote.reset()
	openContext, cancel := context.WithCancel(testContext(t))
	r, err := Open(openContext, f.cat, f.newObjects(t, "guide"), Selection{"guide", "generation-a", "one"}, testLimits())
	if err != nil {
		t.Fatal("strict published read required receipt trust", err)
	}
	cancel()
	f.pool.Close()
	got, err := r.Search(testContext(t), prepared(t, "retry"), queryLimits())
	if err != nil || len(got.Hits) != 2 {
		t.Fatal("open context/catalog lifetime retained", err)
	}
	quote, err := r.Quote(testContext(t), got.Hits[0].Ref, 4096)
	if err != nil || !strings.Contains(quote, "Retry uploads safely.") {
		t.Fatal(quote, err)
	}
	if !reflect.DeepEqual(f.remote.counts(), map[string]int{"GET": 3}) {
		t.Fatal("receipt work or extra I/O", f.remote.counts())
	}
	t.Log("quarantined receipts, canceled opening context and closed catalog: search/strict quote PASS; GET=3 HEAD=0 PUT=0")
}

func TestReaderAbsentEmptyAndUnpublishedSelections(t *testing.T) {
	f := setup(t)
	b := makeBundle(t, "generation-a", "Retry uploads safely.")
	f.publish(t, b)
	cases := []struct {
		name, generation, edition string
		want                      error
		gets                      int
	}{
		{"missing generation", "absent", "one", catalog.ErrNotFound, 0},
		{"unpublished generation", "candidate", "one", catalog.ErrNotFound, 0},
		{"missing edition", "generation-a", "absent", gm.ErrNotFound, 1},
		{"known empty", "generation-a", "empty", nil, 2},
	}
	if _, err := f.cat.Bind(testContext(t), catalog.Request{Collection: "guide", Key: "unfinished", FingerprintVersion: "synthetic/v1"}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f.remote.reset()
			r, err := Open(testContext(t), f.cat, f.objects, Selection{"guide", tc.generation, tc.edition}, testLimits())
			if !errors.Is(err, tc.want) || tc.want != nil && r != nil {
				t.Fatal(r, err)
			}
			if tc.want == nil {
				q := prepared(t, "retry")
				q.Edition = "empty"
				got, err := r.Search(testContext(t), q, queryLimits())
				if err != nil || got.RankingVersion != termsearch.BM25Version || len(got.Hits) != 0 {
					t.Fatal("known-empty result", got, err)
				}
			}
			if f.remote.counts()["GET"] != tc.gets {
				t.Fatal("unexpected reads", f.remote.counts())
			}
		})
	}
	// An empty edition is not an excuse to omit its declared object.
	emptyPath := f.path(t, b.description.Editions[1].Index)
	f.remote.mu.Lock()
	delete(f.remote.bodies, emptyPath)
	f.remote.mu.Unlock()
	if r, err := Open(testContext(t), f.cat, f.objects, Selection{"guide", "generation-a", "empty"}, testLimits()); !errors.Is(err, objectstore.ErrNotFound) || r != nil {
		t.Fatal("missing empty index accepted", err)
	}
}

func TestReaderPreIOBoundsAndPhysicalBinding(t *testing.T) {
	f := setup(t)
	b := makeBundle(t, "generation-a", "Retry uploads safely.")
	f.publish(t, b)
	selected := Selection{"guide", "generation-a", "one"}
	for _, tc := range []struct {
		name   string
		change func(*Limits)
		gets   int
	}{
		{"manifest bytes", func(l *Limits) { l.Manifest.MaxEncodedBytes = len(b.manifest) - 1 }, 0},
		{"index bytes", func(l *Limits) { l.Edition.MaxEncodedBytes = int(b.description.Editions[0].Index.Bytes) - 1 }, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			l := testLimits()
			tc.change(&l)
			f.remote.reset()
			if r, err := Open(testContext(t), f.cat, f.objects, selected, l); !errors.Is(err, ErrLimit) || r != nil {
				t.Fatal(r, err)
			}
			if f.remote.counts()["GET"] != tc.gets {
				t.Fatal("download before admission", f.remote.counts())
			}
		})
	}
	l := testLimits()
	l.Article.MaxEncodedBytes = len(b.article) - 1
	r, err := Open(testContext(t), f.cat, f.objects, selected, l)
	if err != nil {
		t.Fatal(err)
	}
	hits, err := r.Search(testContext(t), prepared(t, "retry"), queryLimits())
	if err != nil {
		t.Fatal(err)
	}
	f.remote.reset()
	if s, err := r.Quote(testContext(t), hits.Hits[0].Ref, 4096); !errors.Is(err, ErrLimit) || s != "" {
		t.Fatal("quote input cap", err)
	}
	if len(f.remote.counts()) != 0 {
		t.Fatal("quote downloaded before cap")
	}
	if r, err := Open(testContext(t), f.cat, f.newObjects(t, "another"), selected, testLimits()); !errors.Is(err, ErrMismatch) || r != nil {
		t.Fatal("collection binding", err)
	}
	if len(f.remote.counts()) != 0 {
		t.Fatal("wrong collection caused I/O")
	}
	if _, err := f.pool.Exec(testContext(t), "UPDATE concordance_imports SET manifest_key='different-key' WHERE generation_id='generation-a'"); err != nil {
		t.Fatal(err)
	}
	if r, err := Open(testContext(t), f.cat, f.objects, selected, testLimits()); !errors.Is(err, ErrMismatch) || r != nil {
		t.Fatal("catalog physical key ignored", err)
	}
	if len(f.remote.counts()) != 0 {
		t.Fatal("wrong key caused I/O")
	}
}

func TestReaderMissingCorruptAndUnavailableObjects(t *testing.T) {
	f := setup(t)
	b := makeBundle(t, "generation-a", "Retry uploads safely.")
	f.publish(t, b)
	r := f.open(t, "generation-a")
	hits, err := r.Search(testContext(t), prepared(t, "retry"), queryLimits())
	if err != nil {
		t.Fatal(err)
	}
	for _, object := range []struct {
		name string
		ref  gm.Ref
	}{{"manifest", ref(b.manifest)}, {"index", b.description.Editions[0].Index}, {"article", ref(b.article)}} {
		path := f.path(t, object.ref)
		for _, corrupt := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/corrupt=%t", object.name, corrupt), func(t *testing.T) {
				f.remote.mu.Lock()
				saved := f.remote.bodies[path]
				if corrupt {
					changed := append([]byte{}, saved...)
					changed[len(changed)-1] ^= 1
					f.remote.bodies[path] = changed
				} else {
					delete(f.remote.bodies, path)
				}
				f.remote.mu.Unlock()
				t.Cleanup(func() { f.remote.mu.Lock(); f.remote.bodies[path] = saved; f.remote.mu.Unlock() })
				want := objectstore.ErrNotFound
				if corrupt {
					want = objectstore.ErrIntegrity
				}
				if object.name == "article" {
					if s, err := r.Quote(testContext(t), hits.Hits[0].Ref, 4096); !errors.Is(err, want) || s != "" {
						t.Fatal("quote accepted invalid bytes", err)
					}
				} else if got, err := Open(testContext(t), f.cat, f.objects, Selection{"guide", "generation-a", "one"}, testLimits()); !errors.Is(err, want) || got != nil {
					t.Fatal("open accepted invalid bytes", err)
				}
			})
		}
	}
	f.server.Close()
	// Loaded search needs no provider even when a later article read cannot finish.
	if got, err := r.Search(testContext(t), prepared(t, "retry"), queryLimits()); err != nil || len(got.Hits) != 2 {
		t.Fatal("local search required provider", err)
	}
	if s, err := r.Quote(testContext(t), hits.Hits[0].Ref, 4096); err == nil || s != "" {
		t.Fatal("unavailable provider accepted")
	}
}

func TestReaderQueryReferenceAndConcurrentOwnership(t *testing.T) {
	f := setup(t)
	f.publish(t, makeBundle(t, "generation-a", "Retry uploads safely."))
	r, other := f.open(t, "generation-a"), f.open(t, "generation-a")
	q := prepared(t, "retry")
	result, err := r.Search(testContext(t), q, queryLimits())
	if err != nil {
		t.Fatal(err)
	}
	f.remote.reset()
	for _, ref := range []Reference{{}, otherRef(t, other, q), {owner: r.data, passage: -1}, {owner: r.data, passage: 999999}} {
		if s, err := r.Quote(testContext(t), ref, 4096); err == nil || s != "" {
			t.Fatal("invalid reference accepted")
		}
	}
	if len(f.remote.counts()) != 0 {
		t.Fatal("invalid reference caused I/O")
	}
	wrongEdition := q
	wrongEdition.Edition = "empty"
	wrongVersion := q
	wrongVersion.TermsVersion = "unsupported"
	empty := q
	empty.Terms = nil
	for _, tc := range []struct {
		q    termsearch.Query
		l    termsearch.Limits
		want error
	}{
		{wrongEdition, queryLimits(), ErrMismatch}, {wrongVersion, queryLimits(), termsearch.ErrIncompatible}, {empty, queryLimits(), termsearch.ErrInvalid}, {q, termsearch.Limits{MaxEditions: 1, MaxPassages: 1000, MaxTerms: 20000, MaxInputBytes: 1 << 20, MaxQueryTerms: 16, MaxMatches: 1}, termsearch.ErrLimit},
	} {
		if got, err := r.Search(testContext(t), tc.q, tc.l); !errors.Is(err, tc.want) || !reflect.DeepEqual(got, Result{}) {
			t.Fatal("query error returned partial result", err)
		}
	}
	ctx, cancel := context.WithCancel(testContext(t))
	cancel()
	if got, err := r.Search(ctx, q, queryLimits()); !errors.Is(err, context.Canceled) || !reflect.DeepEqual(got, Result{}) {
		t.Fatal("canceled search", err)
	}
	for _, zero := range []*Reader{nil, {}} {
		if got, err := zero.Search(testContext(t), q, queryLimits()); !errors.Is(err, ErrInvalid) || !reflect.DeepEqual(got, Result{}) {
			t.Fatal("zero reader", err)
		}
		if s, err := zero.Quote(testContext(t), Reference{}, 4096); !errors.Is(err, ErrInvalid) || s != "" {
			t.Fatal("zero quote", err)
		}
	}
	if got, err := r.Search(context.Background(), q, queryLimits()); !errors.Is(err, ErrInvalid) || !reflect.DeepEqual(got, Result{}) {
		t.Fatal("unbounded query", err)
	}
	if got, err := r.Search(nil, q, queryLimits()); !errors.Is(err, ErrInvalid) || !reflect.DeepEqual(got, Result{}) {
		t.Fatal("nil context", err)
	}
	if s, err := r.Quote(testContext(t), result.Hits[0].Ref, 1); !errors.Is(err, htmledition.ErrLimit) || s != "" {
		t.Fatal("quote output limit", err)
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 4 {
				got, err := r.Search(testContext(t), q, queryLimits())
				if err != nil || !reflect.DeepEqual(got, result) {
					t.Error("concurrent search", err)
					return
				}
				s, err := r.Quote(testContext(t), got.Hits[0].Ref, 4096)
				if err != nil || !strings.Contains(s, "Retry uploads safely.") {
					t.Error("concurrent quote", err)
					return
				}
			}
		})
	}
	wg.Wait()
	result.Hits[0] = Hit{}
	if got, err := r.Search(testContext(t), q, queryLimits()); err != nil || got.Hits[0].Ref.owner == nil {
		t.Fatal("result storage retained", err)
	}
}
func otherRef(t testing.TB, r *Reader, q termsearch.Query) Reference {
	t.Helper()
	result, err := r.Search(testContext(t), q, queryLimits())
	if err != nil {
		t.Fatal(err)
	}
	return result.Hits[0].Ref
}
