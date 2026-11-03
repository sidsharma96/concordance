package publishedsearch

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/sidsharma96/concordance/internal/htmlarticlejson"
	"github.com/sidsharma96/concordance/internal/htmlpassage"
	"github.com/sidsharma96/concordance/internal/objectstore"
	"github.com/sidsharma96/concordance/internal/termsearch"
)

func candidateLimits() CandidateLimits {
	return CandidateLimits{
		Evidence:        EvidenceLimits{Passages: htmlpassage.Limits{MaxBlocks: 1000, MaxGroups: 1000, MaxInputBytes: 1 << 20, MaxPassages: 1000, MaxBodyBytes: 4096, MaxReferences: 10000, MaxContextBytes: 1 << 20}, MaxQuoteBytes: 4096, MaxContextBytes: 4096},
		MaxArticleBytes: 2 << 20, MaxTextBytes: 1 << 20,
	}
}

func TestCandidatesPreserveExactBodyContextAndReader(t *testing.T) {
	f := setup(t)
	b := makeHTMLBundle(t, "generation-a", "Delay startup.", `<main class="fixture"><h1>Pods</h1><p>Overview.</p><h2>Checks</h2><p>Intro.</p><div class="notice" title="SlowStart"><p>Delay startup.</p><pre>startupProbe: enabled</pre></div><p>Later.</p></main>`)
	f.publish(t, b)
	r, q := f.open(t, "generation-a"), prepared(t, "delay missingword")
	all, err := r.Search(testContext(t), q, queryLimits())
	if err != nil || len(all.Hits) != 0 {
		t.Fatal("match-all changed", all, err)
	}
	oracle, err := b.index.SearchAny(testContext(t), q, queryLimits())
	if err != nil {
		t.Fatal(err)
	}
	f.remote.reset()
	got, err := r.SearchAnyEvidenceFirst(testContext(t), q, queryLimits(), candidateLimits(), 1)
	if err != nil || got.RankingVersion != termsearch.AnyBM25Version || got.TotalMatches != 2 || len(got.Candidates) != 1 {
		t.Fatal("wrong candidate result", got, err)
	}
	c := got.Candidates[0]
	if c.Ref.passage != oracle.Matches[0].Passage || c.Score != oracle.Matches[0].Score || c.Evidence.Quote != "Delay startup.\nstartupProbe: enabled" || !slices.Equal(c.Evidence.Headings, []string{"Pods", "Checks"}) || !slices.Equal(c.Evidence.Labels, []string{"SlowStart"}) {
		t.Fatal("body/context/rank drift", c)
	}
	if !reflect.DeepEqual(f.remote.counts(), map[string]int{"GET": 1}) {
		t.Fatal("extra article read", f.remote.counts())
	}
	// Compare context to the canonical builder over the independently decoded body.
	a, err := htmlarticlejson.Decode(testContext(t), b.article, testLimits().Article)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := htmlpassage.Build(testContext(t), a, candidateLimits().Evidence.Passages)
	if err != nil {
		t.Fatal(err)
	}
	loc, err := r.Location(testContext(t), c.Ref)
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, p := range plan.Passages {
		if p.Body == loc.Body {
			found++
			for i, h := range p.Headings {
				if c.Evidence.Headings[i] != a.Blocks[h].Text {
					t.Fatal("heading mismatch")
				}
			}
		}
	}
	if found != 1 {
		t.Fatal("canonical range absent")
	}
	// Returned context slices and candidate slices do not mutate later results.
	got.Candidates[0].Evidence.Headings[0] = "edited"
	again, err := r.SearchAnyEvidenceFirst(testContext(t), q, queryLimits(), candidateLimits(), 1)
	if err != nil || again.Candidates[0].Evidence.Headings[0] != "Pods" {
		t.Fatal("owned context alias escaped", err)
	}
	other := f.open(t, "generation-a")
	f.remote.reset()
	if evidence, err := other.Evidence(testContext(t), c.Ref, candidateLimits().Evidence); !errors.Is(err, ErrInvalid) || !reflect.DeepEqual(evidence, Evidence{}) || len(f.remote.counts()) != 0 {
		t.Fatal("cross-reader evidence admitted", evidence, err)
	}
	for _, change := range []func(*EvidenceLimits){
		func(l *EvidenceLimits) { l.MaxContextBytes = 3 },
		func(l *EvidenceLimits) { l.Passages.MaxBodyBytes = 1 },
	} {
		l := candidateLimits().Evidence
		change(&l)
		f.remote.reset()
		if evidence, err := r.Evidence(testContext(t), c.Ref, l); err == nil || !reflect.DeepEqual(evidence, Evidence{}) || f.remote.counts()["GET"] != 1 {
			t.Fatal("late context/plan failure exposed body", evidence, err)
		}
	}
	t.Log("any-word prefix: selected=1 complete=2 GET=1; body excludes context; headings=Pods,Checks notice=SlowStart; cross-reader reference rejected before GET")
}

func TestCandidatePreflightAndInclusiveBudgets(t *testing.T) {
	f := setup(t)
	b := makeBundle(t, "generation-a", "Retry uploads safely.")
	f.publish(t, b)
	r, q := f.open(t, "generation-a"), prepared(t, "retry absent")
	l := candidateLimits()
	l.MaxArticleBytes = int64(2 * len(b.article))
	// Both logical paths share bytes, but this read plan fetches them twice.
	f.remote.reset()
	got, err := r.SearchAnyEvidenceFirst(testContext(t), q, queryLimits(), l, 2)
	if err != nil || len(got.Candidates) != 2 || f.remote.counts()["GET"] != 2 {
		t.Fatal("inclusive read budget", got, err, f.remote.counts())
	}
	f.remote.reset()
	l.MaxArticleBytes--
	got, err = r.SearchAnyEvidenceFirst(testContext(t), q, queryLimits(), l, 2)
	if !errors.Is(err, ErrLimit) || !reflect.DeepEqual(got, CandidateResult{}) || len(f.remote.counts()) != 0 {
		t.Fatal("read budget was not preflighted", got, err, f.remote.counts())
	}
	l = candidateLimits()
	baseline, err := r.SearchAnyEvidenceFirst(testContext(t), q, queryLimits(), l, 2)
	if err != nil {
		t.Fatal(err)
	}
	size := 0
	for _, c := range baseline.Candidates {
		size += len(c.Evidence.Quote)
		for _, text := range append(slices.Clone(c.Evidence.Headings), c.Evidence.Labels...) {
			size += len(text)
		}
	}
	l.MaxTextBytes = size
	if _, err := r.SearchAnyEvidenceFirst(testContext(t), q, queryLimits(), l, 2); err != nil {
		t.Fatal("inclusive text budget", err)
	}
	l.MaxTextBytes--
	if got, err := r.SearchAnyEvidenceFirst(testContext(t), q, queryLimits(), l, 2); !errors.Is(err, ErrLimit) || !reflect.DeepEqual(got, CandidateResult{}) {
		t.Fatal("aggregate text leaked partial result", got, err)
	}
	f.remote.reset()
	search := queryLimits()
	search.MaxMatches = 1
	if got, err := r.SearchAnyEvidenceFirst(testContext(t), q, search, candidateLimits(), 1); !errors.Is(err, termsearch.ErrLimit) || !reflect.DeepEqual(got, CandidateResult{}) || len(f.remote.counts()) != 0 {
		t.Fatal("prefix bypassed complete union admission", got, err)
	}
	for _, limit := range []int{0, -1} {
		if got, err := r.SearchAnyEvidenceFirst(testContext(t), q, queryLimits(), candidateLimits(), limit); !errors.Is(err, ErrInvalid) || !reflect.DeepEqual(got, CandidateResult{}) {
			t.Fatal("nonpositive prefix admitted", got, err)
		}
	}
	for _, change := range []func(*CandidateLimits){
		func(l *CandidateLimits) { l.MaxArticleBytes = 0 },
		func(l *CandidateLimits) { l.MaxTextBytes = 0 },
		func(l *CandidateLimits) { l.Evidence.Passages.MaxReferences = 0 },
		func(l *CandidateLimits) { l.Evidence.MaxContextBytes = 0 },
	} {
		l := candidateLimits()
		change(&l)
		if got, err := r.SearchAnyEvidenceFirst(testContext(t), q, queryLimits(), l, 1); !errors.Is(err, ErrInvalid) || !reflect.DeepEqual(got, CandidateResult{}) || len(f.remote.counts()) != 0 {
			t.Fatal("invalid budgets performed I/O", got, err)
		}
	}
	empty := prepared(t, "missing")
	if got, err := r.SearchAnyEvidenceFirst(testContext(t), empty, queryLimits(), candidateLimits(), int(^uint(0)>>1)); err != nil || got.TotalMatches != 0 || len(got.Candidates) != 0 || got.RankingVersion != termsearch.AnyBM25Version || len(f.remote.counts()) != 0 {
		t.Fatal("empty result performed I/O or allocated by huge limit", got, err)
	}
	t.Log("two shared-body reads charged twice; one byte below plan rejects with GET=0; complete union overflow rejects before GET even for limit=1")
}

func TestCandidateLateReadAndCancellationReturnZero(t *testing.T) {
	f := setup(t)
	f.publish(t, makeBundle(t, "generation-a", "Retry uploads safely."))
	r, q := f.open(t, "generation-a"), prepared(t, "retry")
	f.remote.reset()
	f.remote.mu.Lock()
	f.remote.failGETAt = 2
	f.remote.mu.Unlock()
	got, err := r.SearchAnyEvidenceFirst(testContext(t), q, queryLimits(), candidateLimits(), 2)
	if !errors.Is(err, objectstore.ErrNotFound) || !reflect.DeepEqual(got, CandidateResult{}) || f.remote.counts()["GET"] != 2 {
		t.Fatal("later failure exposed earlier candidate", got, err)
	}
	f.remote.mu.Lock()
	f.remote.failGETAt = 0
	f.remote.mu.Unlock()
	ctx, cancel := context.WithCancel(testContext(t))
	defer cancel()
	client := f.server.Client()
	previous := client.Transport
	defer func() { client.Transport = previous }()
	var calls atomic.Int32
	client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		response, err := previous.RoundTrip(req)
		if calls.Add(1) == 2 {
			cancel()
		}
		return response, err
	})
	got, err = r.SearchAnyEvidenceFirst(ctx, q, queryLimits(), candidateLimits(), 2)
	if !errors.Is(err, context.Canceled) || !reflect.DeepEqual(got, CandidateResult{}) || calls.Load() != 2 {
		t.Fatal("cancellation exposed prefix", got, err)
	}
}
