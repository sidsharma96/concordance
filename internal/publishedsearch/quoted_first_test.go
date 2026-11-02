package publishedsearch

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/sidsharma96/concordance/internal/htmledition"
	"github.com/sidsharma96/concordance/internal/objectstore"
)

func TestQuotedFirstSelectsOneRankingAndOwnsItsOutput(t *testing.T) {
	f := setup(t)
	f.publish(t, makeBundle(t, "generation-a", "Retry uploads safely."))
	r, q := f.open(t, "generation-a"), prepared(t, "retry")
	ranked, err := r.Search(testContext(t), q, queryLimits())
	if err != nil {
		t.Fatal(err)
	}
	all, err := r.SearchQuoted(testContext(t), q, queryLimits(), QuoteLimits{4096, 8192})
	if err != nil {
		t.Fatal(err)
	}
	f.publish(t, makeBundle(t, "generation-b", "Retry requests carefully."))
	f.remote.reset()
	first, err := r.SearchQuotedFirst(testContext(t), q, queryLimits(), QuoteLimits{4096, 8192}, 1)
	if err != nil || first.TotalMatches != 2 || len(first.Hits) != 1 || first.RankingVersion != all.RankingVersion || first.Hits[0] != all.Hits[0] {
		t.Fatal("prefix, count or score changed", first, err)
	}
	if !reflect.DeepEqual(f.remote.counts(), map[string]int{"GET": 1}) {
		t.Fatal("unselected hit was read", f.remote.counts())
	}
	loc, err := r.Location(testContext(t), first.Hits[0].Ref)
	if err != nil || loc.Selection.Generation != "generation-a" {
		t.Fatal("batch followed later publication", loc, err)
	}
	maxInt := int(^uint(0) >> 1)
	large, err := r.SearchQuotedFirst(testContext(t), q, queryLimits(), QuoteLimits{4096, 8192}, maxInt)
	if err != nil || !reflect.DeepEqual(large, all) {
		t.Fatal("limit above match count allocated or changed output", large, err)
	}
	first.Hits[0] = QuotedHit{}
	if all.Hits[0].Ref != ranked.Hits[0].Ref || large.Hits[0].Ref != ranked.Hits[0].Ref {
		t.Fatal("returned slices alias other results or ranking")
	}
	t.Log("first batch: shown=1, indexed matches=2, article GETs=1; original-generation score/reference/quote preserved")
}

func TestQuotedFirstBudgetsStillAdmitCompleteMatches(t *testing.T) {
	f := setup(t)
	f.publish(t, makeBundle(t, "generation-a", "Retry uploads safely."))
	r, q := f.open(t, "generation-a"), prepared(t, "retry")
	f.remote.reset()
	for _, limit := range []int{0, -1} {
		got, err := r.SearchQuotedFirst(testContext(t), q, queryLimits(), QuoteLimits{26, 26}, limit)
		if !errors.Is(err, ErrInvalid) || !reflect.DeepEqual(got, QuotedResult{}) {
			t.Fatal("nonpositive quote count admitted", got, err)
		}
	}
	tooFew := queryLimits()
	tooFew.MaxMatches = 1
	got, err := r.SearchQuotedFirst(testContext(t), q, tooFew, QuoteLimits{26, 26}, 1)
	if err == nil || !reflect.DeepEqual(got, QuotedResult{}) || len(f.remote.counts()) != 0 {
		t.Fatal("prefix bypassed complete-match admission", got, err, f.remote.counts())
	}
	got, err = r.SearchQuotedFirst(testContext(t), q, queryLimits(), QuoteLimits{26, 26}, 1)
	if err != nil || got.TotalMatches != 2 || len(got.Hits) != 1 || got.Hits[0].Quote != "Pods\nRetry uploads safely." {
		t.Fatal("selected byte limit charged unselected quotes", got, err)
	}
	for _, tc := range []struct {
		quotes QuoteLimits
		limit  int
		want   error
	}{
		{QuoteLimits{25, 52}, 1, htmledition.ErrLimit},
		{QuoteLimits{26, 51}, 2, ErrLimit},
		{QuoteLimits{}, 1, ErrInvalid},
	} {
		got, err := r.SearchQuotedFirst(testContext(t), q, queryLimits(), tc.quotes, tc.limit)
		if !errors.Is(err, tc.want) || !reflect.DeepEqual(got, QuotedResult{}) {
			t.Fatal("selected quote budget exposed partial output", got, err)
		}
	}
	empty := prepared(t, "unmatched")
	f.remote.reset()
	got, err = r.SearchQuotedFirst(testContext(t), empty, queryLimits(), QuoteLimits{1, 1}, 1)
	if err != nil || got.TotalMatches != 0 || len(got.Hits) != 0 || got.RankingVersion == "" || len(f.remote.counts()) != 0 {
		t.Fatal("empty result changed or read an article", got, err)
	}
}

func TestQuotedFirstDoesNotReadUnselectedFailure(t *testing.T) {
	f := setup(t)
	f.publish(t, makeBundle(t, "generation-a", "Retry uploads safely."))
	r, q := f.open(t, "generation-a"), prepared(t, "retry")
	f.remote.reset()
	f.remote.mu.Lock()
	f.remote.failGETAt = 2
	f.remote.mu.Unlock()
	got, err := r.SearchQuotedFirst(testContext(t), q, queryLimits(), QuoteLimits{4096, 8192}, 1)
	if err != nil || got.TotalMatches != 2 || len(got.Hits) != 1 {
		t.Fatal("unselected failure affected selected batch", got, err)
	}
	f.remote.reset()
	got, err = r.SearchQuotedFirst(testContext(t), q, queryLimits(), QuoteLimits{4096, 8192}, 2)
	if !errors.Is(err, objectstore.ErrNotFound) || !reflect.DeepEqual(got, QuotedResult{}) {
		t.Fatal("selected later failure exposed earlier success", got, err)
	}
	if !reflect.DeepEqual(f.remote.counts(), map[string]int{"GET": 2}) {
		t.Fatal("failure did not follow first successful quote", f.remote.counts())
	}
	t.Log("same missing second GET: limit=1 succeeds without reading it; limit=2 returns an empty result")
}

func TestQuotedFirstCancellationDiscardsSelectedPrefix(t *testing.T) {
	f := setup(t)
	f.publish(t, makeBundle(t, "generation-a", "Retry uploads safely."))
	r, q := f.open(t, "generation-a"), prepared(t, "retry")
	ctx, cancel := context.WithCancel(testContext(t))
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
	got, err := r.SearchQuotedFirst(ctx, q, queryLimits(), QuoteLimits{4096, 8192}, 2)
	if !errors.Is(err, context.Canceled) || !reflect.DeepEqual(got, QuotedResult{}) || requests.Load() != 2 {
		t.Fatal("cancellation exposed a selected prefix", got, err)
	}
}
