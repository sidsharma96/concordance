package publishedsearch

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/sidsharma96/concordance/internal/htmledition"
	"github.com/sidsharma96/concordance/internal/objectstore"
	"github.com/sidsharma96/concordance/internal/termsearch"
)

func TestQuotedSearchPreservesCompleteRankingAndPinnedQuotes(t *testing.T) {
	f := setup(t)
	a := makeBundle(t, "generation-a", "Retry uploads safely.")
	f.publish(t, a)
	r := f.open(t, "generation-a")
	q := prepared(t, "retry")
	ranked, err := r.Search(testContext(t), q, queryLimits())
	if err != nil || len(ranked.Hits) != 2 {
		t.Fatal(ranked, err)
	}
	text, err := r.Quote(testContext(t), ranked.Hits[0].Ref, 4096)
	if err != nil {
		t.Fatal(err)
	}
	l := QuoteLimits{MaxQuoteBytes: len(text), MaxTotalQuoteBytes: 2 * len(text)}
	f.publish(t, makeBundle(t, "generation-b", "Retry requests carefully."))
	f.remote.reset()
	got, err := r.SearchQuoted(testContext(t), q, queryLimits(), l)
	if err != nil || len(got.Hits) != 2 || got.RankingVersion != ranked.RankingVersion {
		t.Fatal(got, err)
	}
	for i, hit := range got.Hits {
		if hit.Ref != ranked.Hits[i].Ref || hit.Score != ranked.Hits[i].Score || hit.Quote != text {
			t.Fatal("ranking or quote changed", i)
		}
		loc, err := r.Location(testContext(t), hit.Ref)
		if err != nil || loc.Selection.Generation != "generation-a" {
			t.Fatal("selection changed", err)
		}
	}
	if !reflect.DeepEqual(f.remote.counts(), map[string]int{"GET": 2}) {
		t.Fatal("must use the existing strict path per hit", f.remote.counts())
	}
	t.Logf("complete batch: %d hits, %d quote bytes, 2 GETs; exact old-generation scores/references/quotes preserved", len(got.Hits), 2*len(text))

	// A separately assembled result owns its slice and quote strings. Retaining
	// Ref still deliberately retains the immutable reader, not the input query.
	other, err := r.SearchQuoted(testContext(t), q, queryLimits(), l)
	if err != nil {
		t.Fatal(err)
	}
	got.Hits[0] = QuotedHit{}
	if other.Hits[0].Quote != text || other.Hits[0].Ref != ranked.Hits[0].Ref {
		t.Fatal("separate results alias mutable hit storage")
	}
	copyReader := *r
	if _, err := copyReader.Location(testContext(t), other.Hits[0].Ref); err != nil {
		t.Fatal(err)
	}
	fresh := f.open(t, "generation-a")
	if _, err := fresh.Location(testContext(t), other.Hits[0].Ref); !errors.Is(err, ErrInvalid) {
		t.Fatal("separate reader accepted a batch handle", err)
	}
}

func TestQuotedSearchDiscardsEarlierSuccessOnLaterReadFailure(t *testing.T) {
	f := setup(t)
	f.publish(t, makeBundle(t, "generation-a", "Retry uploads safely."))
	r := f.open(t, "generation-a")
	f.remote.reset()
	f.remote.mu.Lock()
	f.remote.failGETAt = 2
	f.remote.mu.Unlock()
	got, err := r.SearchQuoted(testContext(t), prepared(t, "retry"), queryLimits(), QuoteLimits{4096, 8192})
	if !errors.Is(err, objectstore.ErrNotFound) || !reflect.DeepEqual(got, QuotedResult{}) {
		t.Fatal("partial batch escaped", got, err)
	}
	if !reflect.DeepEqual(f.remote.counts(), map[string]int{"GET": 2}) {
		t.Fatal("fault did not occur after one successful body read", f.remote.counts())
	}
	t.Log("second quote GET fails after first succeeds: returned hits=0, ranking version empty, wrapped missing-object error")
}

func TestQuotedSearchByteCeilingsAndCorruption(t *testing.T) {
	f := setup(t)
	b := makeBundle(t, "generation-a", "Retry uploads safely.")
	f.publish(t, b)
	r, q := f.open(t, "generation-a"), prepared(t, "retry")
	ranked, err := r.Search(testContext(t), q, queryLimits())
	if err != nil || len(ranked.Hits) != 2 {
		t.Fatal(ranked, err)
	}
	text, err := r.Quote(testContext(t), ranked.Hits[0].Ref, 4096)
	if err != nil {
		t.Fatal(err)
	}
	size := len(text)
	if got, err := r.SearchQuoted(testContext(t), q, queryLimits(), QuoteLimits{size, 2 * size}); err != nil || len(got.Hits) != 2 {
		t.Fatal("inclusive exact ceiling", got, err)
	}
	for _, tc := range []struct {
		name string
		l    QuoteLimits
		want error
	}{
		{"one quote", QuoteLimits{size - 1, 2 * size}, htmledition.ErrLimit},
		{"aggregate", QuoteLimits{size, 2*size - 1}, ErrLimit},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := r.SearchQuoted(testContext(t), q, queryLimits(), tc.l)
			if !errors.Is(err, tc.want) || !reflect.DeepEqual(got, QuotedResult{}) {
				t.Fatal("overflow truncated results", got, err)
			}
		})
	}
	// A large caller budget is not an allocation request. Remaining-capacity
	// arithmetic also stays valid near the platform's largest signed integer.
	maxInt := int(^uint(0) >> 1)
	if got, err := r.SearchQuoted(testContext(t), q, queryLimits(), QuoteLimits{maxInt, maxInt}); err != nil || len(got.Hits) != 2 {
		t.Fatal(got, err)
	}
	f.remote.mu.Lock()
	body := f.remote.bodies[f.path(t, ref(b.article))]
	body[len(body)/2] ^= 1
	f.remote.mu.Unlock()
	got, err := r.SearchQuoted(testContext(t), q, queryLimits(), QuoteLimits{4096, 8192})
	if !errors.Is(err, objectstore.ErrIntegrity) || !reflect.DeepEqual(got, QuotedResult{}) {
		t.Fatal("corrupt bytes escaped", got, err)
	}
	t.Logf("quote=%q (%d bytes): total %d succeeds, total %d fails without partial hits; corruption is rejected", text, size, 2*size, 2*size-1)
}

func TestQuotedSearchAdmissionEmptyAndCancellation(t *testing.T) {
	f := setup(t)
	f.publish(t, makeBundle(t, "generation-a", "Retry uploads safely."))
	r, q := f.open(t, "generation-a"), prepared(t, "retry")
	ctx, cancel := context.WithCancel(testContext(t))
	cancel()
	f.remote.reset()
	wrong := q
	wrong.Edition = "absent"
	tooFew := queryLimits()
	tooFew.MaxMatches = 1
	for _, tc := range []struct {
		ctx context.Context
		r   *Reader
		q   termsearch.Query
		s   termsearch.Limits
		l   QuoteLimits
	}{
		{nil, r, q, queryLimits(), QuoteLimits{1, 1}},
		{context.Background(), r, q, queryLimits(), QuoteLimits{1, 1}},
		{ctx, r, q, queryLimits(), QuoteLimits{1, 1}},
		{testContext(t), nil, q, queryLimits(), QuoteLimits{1, 1}},
		{testContext(t), r, q, queryLimits(), QuoteLimits{}},
		{testContext(t), r, q, queryLimits(), QuoteLimits{1, 0}},
		{testContext(t), r, q, queryLimits(), QuoteLimits{-1, 1}},
		{testContext(t), r, wrong, queryLimits(), QuoteLimits{1, 1}},
		{testContext(t), r, q, tooFew, QuoteLimits{4096, 8192}},
	} {
		got, err := tc.r.SearchQuoted(tc.ctx, tc.q, tc.s, tc.l)
		if err == nil || !reflect.DeepEqual(got, QuotedResult{}) {
			t.Fatal("invalid request accepted", got, err)
		}
	}
	if len(f.remote.counts()) != 0 {
		t.Fatal("invalid request performed article I/O")
	}
	empty := prepared(t, "unmatched")
	got, err := r.SearchQuoted(testContext(t), empty, queryLimits(), QuoteLimits{1, 1})
	if err != nil || len(got.Hits) != 0 || got.RankingVersion == "" || len(f.remote.counts()) != 0 {
		t.Fatal("empty result performed I/O or lost success metadata", got, err)
	}
	knownEmpty, err := Open(testContext(t), f.cat, f.objects, Selection{"guide", "generation-a", "empty"}, testLimits())
	if err != nil {
		t.Fatal(err)
	}
	empty.Edition = "empty"
	f.remote.reset()
	if got, err := knownEmpty.SearchQuoted(testContext(t), empty, queryLimits(), QuoteLimits{1, 1}); err != nil || len(got.Hits) != 0 || len(f.remote.counts()) != 0 {
		t.Fatal("known-empty edition", got, err)
	}

	// Cancel exactly when the second quote requests a body: the first quote has
	// completed. The transport performs no second network read or timing sleep.
	ctx, cancel = context.WithCancel(testContext(t))
	defer cancel()
	client := f.server.Client()
	previous := client.Transport
	var requests atomic.Int32
	client.Transport = quoteRoundTripper(func(req *http.Request) (*http.Response, error) {
		if req.Method == "GET" && requests.Add(1) == 2 {
			cancel()
			return nil, context.Canceled
		}
		return previous.RoundTrip(req)
	})
	t.Cleanup(func() { client.Transport = previous })
	got, err = r.SearchQuoted(ctx, q, queryLimits(), QuoteLimits{4096, 8192})
	if !errors.Is(err, context.Canceled) || !reflect.DeepEqual(got, QuotedResult{}) || requests.Load() != 2 {
		t.Fatal("cancellation exposed earlier quote", got, err, requests.Load())
	}
}

type quoteRoundTripper func(*http.Request) (*http.Response, error)

func (f quoteRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestQuotedSearchConcurrentCallsOwnSeparateResults(t *testing.T) {
	f := setup(t)
	f.publish(t, makeBundle(t, "generation-a", "Retry uploads safely."))
	r, q := f.open(t, "generation-a"), prepared(t, "retry")
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			got, err := r.SearchQuoted(testContext(t), q, queryLimits(), QuoteLimits{4096, 8192})
			if err != nil || len(got.Hits) != 2 || !strings.Contains(got.Hits[0].Quote, "Retry uploads safely.") {
				t.Error("concurrent complete batch", err)
				return
			}
			got.Hits[0] = QuotedHit{}
		})
	}
	wg.Wait()
}
