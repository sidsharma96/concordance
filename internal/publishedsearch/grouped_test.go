package publishedsearch

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"reflect"
	"slices"
	"sync"
	"sync/atomic"
	"testing"

	gm "github.com/sidsharma96/concordance/internal/generationmanifest"
	"github.com/sidsharma96/concordance/internal/htmledition"
	"github.com/sidsharma96/concordance/internal/htmlpassage"
	"github.com/sidsharma96/concordance/internal/htmlterms"
	"github.com/sidsharma96/concordance/internal/objectstore"
	"github.com/sidsharma96/concordance/internal/termsearch"
	"github.com/sidsharma96/concordance/internal/textterms"
)

func groupedLimits() GroupedCandidateLimits {
	l := candidateLimits()
	return GroupedCandidateLimits{Evidence: l.Evidence, MaxDistinctArticleBytes: l.MaxArticleBytes, MaxTextBytes: l.MaxTextBytes}
}

// Three distinct logical paths have tied matches in A,B,A order. This exercises
// grouping across separated ranks, including two source slots sharing exact bytes.
func makeGroupedBundle(t testing.TB) bundle {
	t.Helper()
	markup := func(word string) string {
		return `<main class="fixture"><h1>Pods</h1><p>Overview.</p><div class="notice" title="Advice"><p>Retry ` + word + ` safely.</p></div><img src="/images/pod.svg" alt="Pod diagram"></main>`
	}
	b := makeHTMLBundle(t, "generation-a", "Retry alpha safely.", markup("alpha"))
	other := makeHTMLBundle(t, "generation-b", "Retry bravo safely.", markup("bravo"))
	available := map[gm.Ref][]byte{}
	for _, body := range append(slices.Clone(b.artifacts), other.artifacts...) {
		available[ref(body)] = body
	}
	e := &b.description.Editions[0]
	file := other.description.Editions[0].Captures[0].Files[0]
	file.Path = "docs/middle.md"
	c := &e.Captures[0]
	c.Files = append(c.Files, file)
	doc := other.description.Editions[0].Documents[0]
	doc.File, doc.NativeID, doc.Location = len(c.Files)-1, file.Path, file.Path
	e.Documents = []gm.Document{e.Documents[0], doc, e.Documents[1]}
	l := testLimits()
	prep := htmlterms.Limits{Passages: candidateLimits().Evidence.Passages, Field: textterms.Limits{MaxInputBytes: 65536, MaxTerms: 10000, MaxTermBytes: 1024, MaxOutputBytes: 1 << 20}, MaxViewBytes: 1 << 20, MaxTerms: 20000, MaxOutputBytes: 1 << 20}
	var err error
	b.index, err = htmledition.Build(testContext(t), "one", [][]byte{b.article, other.article, b.article}, htmledition.BuildLimits{File: l.Edition, Article: l.Article, Preparation: prep})
	if err != nil {
		t.Fatal(err)
	}
	wire, err := htmledition.Encode(testContext(t), b.index, l.Edition)
	if err != nil {
		t.Fatal(err)
	}
	e.Index = ref(wire)
	available[ref(wire)] = wire
	b.encode(t)
	m, err := gm.Open(testContext(t), b.manifest, ref(b.manifest), "guide", "generation-a", l.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	objects, err := m.Objects(testContext(t))
	if err != nil {
		t.Fatal(err)
	}
	b.artifacts = nil
	for _, object := range objects {
		body, ok := available[object]
		if !ok {
			t.Fatal("missing synthetic artifact")
		}
		b.artifacts = append(b.artifacts, body)
	}
	return b
}

func TestGroupedCandidatesMatchUncachedOrderAndOwnOutput(t *testing.T) {
	f := setup(t)
	b := makeGroupedBundle(t)
	f.publish(t, b)
	r, q := f.open(t, "generation-a"), prepared(t, "retry absent")
	f.remote.reset()
	want, err := r.SearchAnyEvidenceFirst(testContext(t), q, queryLimits(), candidateLimits(), 3)
	if err != nil || len(want.Candidates) != 3 || f.remote.counts()["GET"] != 3 {
		t.Fatal("uncached oracle", want, err, f.remote.counts())
	}
	paths := []string{"docs/pods.md", "docs/middle.md", "docs/other.md"}
	for i, hit := range want.Candidates {
		location, err := r.Location(testContext(t), hit.Ref)
		if err != nil || location.Document.NativeID != paths[i] || hit.Score != want.Candidates[0].Score {
			t.Fatal("fixture must interleave articles in tied rank order", want, err)
		}
	}
	f.remote.reset()
	got, err := r.SearchAnyEvidenceGroupedFirst(testContext(t), q, queryLimits(), groupedLimits(), 3)
	if err != nil || !reflect.DeepEqual(got, want) || !reflect.DeepEqual(f.remote.counts(), map[string]int{"GET": 2}) {
		t.Fatal("grouped result/order/read mismatch", got, err, f.remote.counts())
	}
	got.Candidates[0].Evidence.Headings[0] = "changed"
	got.Candidates[0].Evidence.Labels[0] = "changed"
	if got.Candidates[2].Evidence.Headings[0] != "Pods" || got.Candidates[2].Evidence.Labels[0] != "Advice" {
		t.Fatal("shared article leaked mutable output slices")
	}
	got.Candidates[0] = Candidate{}
	f.remote.reset()
	again, err := r.SearchAnyEvidenceGroupedFirst(testContext(t), q, queryLimits(), groupedLimits(), 3)
	if err != nil || !reflect.DeepEqual(again, want) || f.remote.counts()["GET"] != 2 {
		t.Fatal("request retained article/output state", again, err, f.remote.counts())
	}
	other := f.open(t, "generation-a")
	f.remote.reset()
	if _, err := other.groupArticleReads(testContext(t), []Hit{{Ref: want.Candidates[0].Ref}}, 1<<20); !errors.Is(err, ErrInvalid) || len(f.remote.counts()) != 0 {
		t.Fatal("cross-reader handle admitted", err, f.remote.counts())
	}
	if _, err := r.groupArticleReads(testContext(t), []Hit{{}}, 1<<20); !errors.Is(err, ErrInvalid) {
		t.Fatal("zero reference admitted", err)
	}
	t.Log("ranked articles=A,B,A; uncached GET=3 grouped GET=2; exact refs/scores/body/headings/labels preserved; next request GET=2")
}

func TestGroupedAllTermsMatchesIndependentEvidence(t *testing.T) {
	f := setup(t)
	f.publish(t, makeGroupedBundle(t))
	r, q := f.open(t, "generation-a"), prepared(t, "retry")
	ranked, err := r.Search(testContext(t), q, queryLimits())
	if err != nil || len(ranked.Hits) != 3 {
		t.Fatal(ranked, err)
	}
	want := CandidateResult{RankingVersion: ranked.RankingVersion, TotalMatches: len(ranked.Hits)}
	f.remote.reset()
	for _, hit := range ranked.Hits {
		evidence, err := r.Evidence(testContext(t), hit.Ref, candidateLimits().Evidence)
		if err != nil {
			t.Fatal(err)
		}
		want.Candidates = append(want.Candidates, Candidate{Ref: hit.Ref, Score: hit.Score, Evidence: evidence})
	}
	if f.remote.counts()["GET"] != 3 {
		t.Fatal("uncached oracle", f.remote.counts())
	}
	for range 2 {
		f.remote.reset()
		got, err := r.SearchEvidenceGroupedFirst(testContext(t), q, queryLimits(), groupedLimits(), queryLimits().MaxMatches)
		if err != nil || !reflect.DeepEqual(got, want) || f.remote.counts()["GET"] != 2 {
			t.Fatal("all-term grouping changed output or retained bytes between calls", got, err, f.remote.counts())
		}
	}
	f.remote.reset()
	got, err := r.SearchEvidenceGroupedFirst(testContext(t), prepared(t, "retry absent"), queryLimits(), groupedLimits(), 3)
	if err != nil || got.TotalMatches != 0 || len(got.Candidates) != 0 || len(f.remote.counts()) != 0 {
		t.Fatal("all-term policy became any-term", got, err, f.remote.counts())
	}
	search := queryLimits()
	search.MaxMatches = 2
	if got, err := r.SearchEvidenceGroupedFirst(testContext(t), q, search, groupedLimits(), 1); !errors.Is(err, termsearch.ErrLimit) || !reflect.DeepEqual(got, CandidateResult{}) || len(f.remote.counts()) != 0 {
		t.Fatal("prefix bypassed complete ranking admission", got, err)
	}
	if _, err := r.SearchEvidenceGroupedFirst(testContext(t), q, queryLimits(), groupedLimits(), 0); !errors.Is(err, ErrInvalid) {
		t.Fatal("invalid prefix", err)
	}
	t.Log("all-term A,B,A: strict per-hit oracle GET=3; grouped GET=2 on each fresh call; exact values/order preserved")
}

func TestGroupedCandidatesPreflightAndTextBudgets(t *testing.T) {
	f := setup(t)
	b := makeGroupedBundle(t)
	f.publish(t, b)
	r, q := f.open(t, "generation-a"), prepared(t, "retry")
	l := groupedLimits()
	l.MaxDistinctArticleBytes = b.description.Editions[0].Documents[0].Article.Bytes + b.description.Editions[0].Documents[1].Article.Bytes
	f.remote.reset()
	baseline, err := r.SearchAnyEvidenceGroupedFirst(testContext(t), q, queryLimits(), l, 3)
	if err != nil || len(baseline.Candidates) != 3 || f.remote.counts()["GET"] != 2 {
		t.Fatal("inclusive distinct bytes", baseline, err, f.remote.counts())
	}
	l.MaxDistinctArticleBytes--
	f.remote.reset()
	if got, err := r.SearchAnyEvidenceGroupedFirst(testContext(t), q, queryLimits(), l, 3); !errors.Is(err, ErrLimit) || !reflect.DeepEqual(got, CandidateResult{}) || len(f.remote.counts()) != 0 {
		t.Fatal("distinct-byte plan was not admitted before GET", got, err, f.remote.counts())
	}
	l = groupedLimits()
	l.MaxTextBytes = 0
	for _, c := range baseline.Candidates {
		l.MaxTextBytes += len(c.Evidence.Quote)
		for _, field := range [][]string{c.Evidence.Headings, c.Evidence.Labels} {
			for _, text := range field {
				l.MaxTextBytes += len(text)
			}
		}
	}
	if got, err := r.SearchAnyEvidenceGroupedFirst(testContext(t), q, queryLimits(), l, 3); err != nil || !reflect.DeepEqual(got, baseline) {
		t.Fatal("inclusive occurrence-weighted output bytes", got, err)
	}
	l.MaxTextBytes--
	if got, err := r.SearchAnyEvidenceGroupedFirst(testContext(t), q, queryLimits(), l, 3); !errors.Is(err, ErrLimit) || !reflect.DeepEqual(got, CandidateResult{}) {
		t.Fatal("aggregate output overflow leaked batch", got, err)
	}
	f.remote.reset()
	search := queryLimits()
	search.MaxMatches = 2
	if got, err := r.SearchAnyEvidenceGroupedFirst(testContext(t), q, search, groupedLimits(), 1); !errors.Is(err, termsearch.ErrLimit) || !reflect.DeepEqual(got, CandidateResult{}) || len(f.remote.counts()) != 0 {
		t.Fatal("prefix bypassed complete union admission", got, err, f.remote.counts())
	}
	for _, change := range []func(*GroupedCandidateLimits){
		func(l *GroupedCandidateLimits) { l.MaxDistinctArticleBytes = 0 },
		func(l *GroupedCandidateLimits) { l.MaxTextBytes = -1 },
		func(l *GroupedCandidateLimits) { l.Evidence.MaxQuoteBytes = 0 },
		func(l *GroupedCandidateLimits) { l.Evidence.Passages.MaxReferences = 0 },
	} {
		l := groupedLimits()
		change(&l)
		if got, err := r.SearchAnyEvidenceGroupedFirst(testContext(t), q, queryLimits(), l, 3); !errors.Is(err, ErrInvalid) || !reflect.DeepEqual(got, CandidateResult{}) || len(f.remote.counts()) != 0 {
			t.Fatal("invalid limits reached I/O", got, err, f.remote.counts())
		}
	}
	for _, limit := range []int{0, -1} {
		if got, err := r.SearchAnyEvidenceGroupedFirst(testContext(t), q, queryLimits(), groupedLimits(), limit); !errors.Is(err, ErrInvalid) || !reflect.DeepEqual(got, CandidateResult{}) {
			t.Fatal("nonpositive prefix admitted", got, err)
		}
	}
	if got, err := r.SearchAnyEvidenceGroupedFirst(testContext(t), prepared(t, "missing"), queryLimits(), groupedLimits(), int(^uint(0)>>1)); err != nil || got.RankingVersion != termsearch.AnyBM25Version || got.TotalMatches != 0 || len(got.Candidates) != 0 || len(f.remote.counts()) != 0 {
		t.Fatal("empty result or huge limit caused I/O", got, err, f.remote.counts())
	}
	t.Log("distinct-byte exact bound accepts; one byte less rejects before GET; output bytes still charge every logical candidate")
}

func TestGroupedCandidatesVerifyEverySharedSlot(t *testing.T) {
	for _, field := range []string{"profile", "image"} {
		t.Run(field, func(t *testing.T) {
			f := setup(t)
			b := makeBundle(t, "generation-a", "Retry uploads safely.")
			// Same article bytes, but inconsistent provenance in the second slot.
			// Normal publication rejects this; the reader must reject it too.
			if field == "profile" {
				b.description.Editions[0].Documents[1].Profile = "wrong/v1"
			} else {
				b.description.Editions[0].Documents[1].Images[0].Block = 0
			}
			b.encode(t)
			f.publishUnchecked(t, b)
			r := f.open(t, "generation-a")
			f.remote.reset()
			got, err := r.SearchAnyEvidenceGroupedFirst(testContext(t), prepared(t, "retry"), queryLimits(), groupedLimits(), 2)
			if !errors.Is(err, gm.ErrMismatch) || !reflect.DeepEqual(got, CandidateResult{}) || f.remote.counts()["GET"] != 1 {
				t.Fatal("shared bytes bypassed second slot verification", got, err, f.remote.counts())
			}
		})
	}
}

func TestGroupedCandidatesLateFailuresDiscardBatch(t *testing.T) {
	for _, failure := range []string{"missing", "hash", "length", "context", "quote", "plan", "per-read"} {
		t.Run(failure, func(t *testing.T) {
			f := setup(t)
			b := makeGroupedBundle(t)
			f.publish(t, b)
			r := f.open(t, "generation-a")
			l := groupedLimits()
			var want error
			wantGET := 2
			path := f.path(t, b.description.Editions[0].Documents[1].Article)
			f.remote.mu.Lock()
			switch failure {
			case "missing":
				delete(f.remote.bodies, path)
				want = objectstore.ErrNotFound
			case "hash":
				body := bytes.Clone(f.remote.bodies[path])
				body[len(body)-1] ^= 1
				f.remote.bodies[path] = body
				want = objectstore.ErrIntegrity
			case "length":
				f.remote.bodies[path] = append(bytes.Clone(f.remote.bodies[path]), ' ')
				want = objectstore.ErrIntegrity
			case "context":
				l.Evidence.MaxContextBytes = 1
				want, wantGET = ErrLimit, 1
			case "quote":
				l.Evidence.MaxQuoteBytes = 1
				want, wantGET = htmledition.ErrLimit, 1
			case "plan":
				l.Evidence.Passages.MaxBodyBytes = 1
				want, wantGET = htmlpassage.ErrLimit, 1
			case "per-read":
				r.data.article.MaxEncodedBytes = 1
				want, wantGET = ErrLimit, 0
			}
			f.remote.mu.Unlock()
			f.remote.reset()
			got, err := r.SearchAnyEvidenceGroupedFirst(testContext(t), prepared(t, "retry"), queryLimits(), l, 3)
			if err == nil || want != nil && !errors.Is(err, want) || !reflect.DeepEqual(got, CandidateResult{}) || f.remote.counts()["GET"] != wantGET {
				t.Fatal("failure exposed partial candidate batch", got, err, f.remote.counts())
			}
		})
	}
}

func TestGroupedCandidatesCancellationAndConcurrentCalls(t *testing.T) {
	f := setup(t)
	b := makeGroupedBundle(t)
	f.publish(t, b)
	r, q := f.open(t, "generation-a"), prepared(t, "retry")
	ctx, cancel := context.WithCancel(testContext(t))
	cancel()
	f.remote.reset()
	if got, err := r.SearchAnyEvidenceGroupedFirst(ctx, q, queryLimits(), groupedLimits(), 3); !errors.Is(err, context.Canceled) || !reflect.DeepEqual(got, CandidateResult{}) || len(f.remote.counts()) != 0 {
		t.Fatal("canceled query performed GET", got, err)
	}
	ctx, cancel = context.WithCancel(testContext(t))
	defer cancel()
	client := f.server.Client()
	previous := client.Transport
	var calls atomic.Int32
	defer func() { client.Transport = previous }()
	client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		response, err := previous.RoundTrip(req)
		if calls.Add(1) == 2 {
			cancel()
		}
		return response, err
	})
	got, err := r.SearchAnyEvidenceGroupedFirst(ctx, q, queryLimits(), groupedLimits(), 3)
	client.Transport = previous
	if !errors.Is(err, context.Canceled) || !reflect.DeepEqual(got, CandidateResult{}) || calls.Load() != 2 {
		t.Fatal("later group cancellation exposed earlier candidates", got, err)
	}
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			got, err := r.SearchAnyEvidenceGroupedFirst(testContext(t), q, queryLimits(), groupedLimits(), 3)
			if err != nil || len(got.Candidates) != 3 {
				t.Error("concurrent grouped query", err)
				return
			}
			got.Candidates[0].Evidence.Headings[0] = "owned by this call"
		})
	}
	wg.Wait()
}
