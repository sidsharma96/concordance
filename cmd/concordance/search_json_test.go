package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/sidsharma96/concordance/internal/catalog"
	gm "github.com/sidsharma96/concordance/internal/generationmanifest"
	"github.com/sidsharma96/concordance/internal/htmledition"
	"github.com/sidsharma96/concordance/internal/htmlterms"
	"github.com/sidsharma96/concordance/internal/publishedsearch"
	"github.com/sidsharma96/concordance/internal/rerank"
	"github.com/sidsharma96/concordance/internal/termsearch"
	"github.com/sidsharma96/concordance/internal/textterms"
)

func decodeSearchJSON(t *testing.T, body []byte) searchJSON {
	t.Helper()
	if bytes.Count(body, []byte{'\n'}) != 1 || !bytes.HasSuffix(body, []byte{'\n'}) {
		t.Fatal("expected one complete compact object plus LF")
	}
	var got searchJSON
	d := json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	if err := d.Decode(&got); err != nil {
		t.Fatal(err)
	}
	if err := d.Decode(new(any)); !errors.Is(err, io.EOF) {
		t.Fatal("trailing output", err)
	}
	return got
}

// A later read must target a distinct object now that one request shares bodies.
// Keep both passages eligible so a failure really follows successful evidence.
func makeDistinctJSONBundle(t *testing.T) bundle {
	t.Helper()
	b := makeBundle(t, "generation-a", "Retry uploads safely.")
	other := makeBundle(t, "generation-b", "Retry requests carefully.")
	available := make(map[gm.Ref][]byte)
	for _, body := range append(slices.Clone(b.artifacts), other.artifacts...) {
		available[ref(body)] = body
	}
	e := &b.description.Editions[0]
	e.Documents[1].HTML = other.description.Editions[0].Documents[0].HTML
	e.Documents[1].Article = ref(other.article)
	l := testLimits()
	prep := htmlterms.Limits{Passages: evidenceLimits(readOptions{article: 1 << 20, passages: 1000}, 65536).Passages, Field: textterms.Limits{MaxInputBytes: 65536, MaxTerms: 10000, MaxTermBytes: 1024, MaxOutputBytes: 1 << 20}, MaxViewBytes: 1 << 20, MaxTerms: 20000, MaxOutputBytes: 1 << 20}
	var err error
	b.index, err = htmledition.Build(testContext(t), "one", [][]byte{b.article, other.article}, htmledition.BuildLimits{File: l.Edition, Article: l.Article, Preparation: prep})
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
			t.Fatal("missing fixture object")
		}
		b.artifacts = append(b.artifacts, body)
	}
	return b
}

func TestSearchJSONVerifiedSourceContextAndPrefix(t *testing.T) {
	f := setup(t)
	b := makeMarkupBundle(t, "generation-a", "Delay startup.", `<main class="fixture"><h1>Pods</h1><p>Overview.</p><h2>Checks</h2><p>Intro.</p><div class="notice" title="SlowStart"><p>Delay &quot;startup&quot; \ café.</p><pre>startupProbe: enabled</pre></div><p>Later.</p></main>`, 2)
	f.publish(t, b)
	activateFixture(t, f, "generation-a")
	opened, closed := 0, 0
	var out bytes.Buffer
	args := []string{"--json", "--limit=1", "kubernetes", "DELAY"}
	err := runSearch(testContext(t), args, &out, environment(configEnvironment()), cliConnector(f, &opened, &closed))
	if err != nil {
		t.Fatal(err)
	}
	got := decodeSearchJSON(t, out.Bytes())
	if got.Format != "concordance.docs-search/v1" || got.Status != "ok" || got.Target != "kubernetes" || got.Selection != (searchJSONSelection{"guide", "generation-a", "one"}) || got.Retrieval.Policy != "all_terms_bm25" || got.Retrieval.TermsVersion != prepared(t, "delay").TermsVersion || got.Retrieval.RankingVersion != termsearch.BM25Version || got.Retrieval.IndexedMatches != 2 || got.Retrieval.RankedCandidates != 2 || got.Retrieval.Model != nil || len(got.Results) != 1 {
		t.Fatal("wrong scope/policy/counts", got)
	}
	h := got.Results[0]
	if h.ID != "r1" || h.Source != (searchJSONSource{"git", "docs", "https://example.invalid/docs.git", strings.Repeat("a", 40), "docs/pods.md", "docs/pods.md", b.description.Editions[0].Captures[0].Files[0].BlobID}) {
		t.Fatal("source identity changed", h.Source)
	}
	e := h.Evidence
	if e.Kind != "retained_html_blocks/v1" || e.ArticleSHA256 != fmt.Sprintf("%x", ref(b.article).SHA256) || e.ArticleBytes != int64(len(b.article)) || e.FirstBlock != 4 || e.AfterBlock != 6 || e.Quote != "Delay \"startup\" \\ café.\nstartupProbe: enabled" || !reflect.DeepEqual(e.Headings, []string{"Pods", "Checks"}) || !reflect.DeepEqual(e.NoticeLabels, []string{"SlowStart"}) {
		t.Fatal("lost exact evidence", e)
	}
	for _, forbidden := range []string{`"Ref"`, `"owner"`, `"score"`, `"query"`, "DELAY", "synthetic-secret", `"model"`} {
		if strings.Contains(out.String(), forbidden) {
			t.Fatal("internal field/query/credential escaped", forbidden)
		}
	}
	if opened != 1 || closed != 1 || f.remote.counts()["GET"] != 3 {
		t.Fatal("extra read/leaked connection", opened, closed, f.remote.counts())
	}
	first := bytes.Clone(out.Bytes())
	out.Reset()
	f.remote.reset()
	args = append([]string{"--json", "--limit=1", "--max-output-bytes=" + fmt.Sprint(len(first))}, "kubernetes", "DELAY")
	if err := runSearch(testContext(t), args, &out, environment(configEnvironment()), cliConnector(f, &opened, &closed)); err != nil || !bytes.Equal(first, out.Bytes()) {
		t.Fatal("inclusive encoded budget", err)
	}
	out.Reset()
	args[2] = "--max-output-bytes=" + fmt.Sprint(len(first)-1)
	if err := runSearch(testContext(t), args, &out, environment(configEnvironment()), cliConnector(f, &opened, &closed)); !errors.Is(err, errOutputLimit) || out.Len() != 0 {
		t.Fatal("encoded overflow escaped", err)
	}
	t.Log("JSON prefix: shown=1 indexed=2 ranked=2 GETs=3; exact Unicode/body/context/source; encoded cap includes LF")
}

func TestSearchJSONAllEmptyAndMissing(t *testing.T) {
	f := setup(t)
	f.publish(t, makeBundle(t, "generation-a", "Retry uploads safely."))
	activateFixture(t, f, "generation-a")
	for _, tc := range []struct {
		name              string
		args              []string
		hits, total, gets int
		fail              bool
	}{
		{"all", []string{"--all", "kubernetes", "retry"}, 2, 2, 3, false},
		{"no matches", []string{"kubernetes", "nonexistent"}, 0, 0, 2, false},
		{"known empty", []string{"--edition=empty", "kubernetes", "retry"}, 0, 0, 2, false},
		{"missing edition", []string{"--edition=missing", "kubernetes", "retry"}, 0, 0, 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			opened, closed := 0, 0
			f.remote.reset()
			err := runSearch(testContext(t), append([]string{"--json"}, tc.args...), &out, environment(configEnvironment()), cliConnector(f, &opened, &closed))
			if tc.fail {
				if err == nil || out.Len() != 0 {
					t.Fatal("error became empty success", err)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				got := decodeSearchJSON(t, out.Bytes())
				if got.Status != "ok" || got.Results == nil || len(got.Results) != tc.hits || got.Retrieval.IndexedMatches != tc.total || got.Retrieval.RankedCandidates != tc.total {
					t.Fatal(got)
				}
				if tc.hits == 2 {
					a, b := got.Results[0], got.Results[1]
					if a.Source.NativeID == b.Source.NativeID || a.Evidence.ArticleSHA256 != b.Evidence.ArticleSHA256 || a.ID != "r1" || b.ID != "r2" || a.Evidence.NoticeLabels == nil {
						t.Fatal("shared content lost document identity")
					}
				}
			}
			if closed != 1 || f.remote.counts()["GET"] != tc.gets {
				t.Fatal("lifetime/requests", closed, f.remote.counts())
			}
		})
	}
}

func TestSearchJSONFailuresHaveNoOutput(t *testing.T) {
	for _, kind := range []string{"later read", "corrupt", "cancel", "match limit", "quote limit", "total limit", "output limit"} {
		t.Run(kind, func(t *testing.T) {
			f := setup(t)
			b := makeBundle(t, "generation-a", "Retry uploads safely.")
			if kind == "later read" || kind == "cancel" {
				b = makeDistinctJSONBundle(t)
			}
			f.publish(t, b)
			activateFixture(t, f, "generation-a")
			ctx, cancel := context.WithCancel(testContext(t))
			defer cancel()
			args := []string{"--json"}
			f.remote.mu.Lock()
			switch kind {
			case "later read":
				f.remote.failGETAt = 4
			case "corrupt":
				f.remote.bodies[f.path(t, ref(b.article))][0] ^= 1
			case "cancel":
				f.remote.onGET = func(n int) {
					if n == 4 {
						cancel()
					}
				}
			case "match limit":
				args = append(args, "--max-matches=1", "--limit=1")
			case "quote limit":
				args = append(args, "--max-quote-bytes=1")
			case "total limit":
				args = append(args, "--max-total-quote-bytes=30")
			case "output limit":
				args = append(args, "--max-output-bytes=1")
			}
			f.remote.mu.Unlock()
			var out bytes.Buffer
			opened, closed := 0, 0
			err := runSearch(ctx, append(args, "kubernetes", "retry"), &out, environment(configEnvironment()), cliConnector(f, &opened, &closed))
			if err == nil || out.Len() != 0 || closed != 1 {
				t.Fatal("partial output or missing cleanup", err, out.String(), closed)
			}
			if kind == "output limit" && !errors.Is(err, errOutputLimit) {
				t.Fatalf("text overflow lost output-limit remedy: %v", err)
			}
			if kind == "match limit" && f.remote.counts()["GET"] != 2 {
				t.Fatal("match overflow fetched body")
			}
			if kind == "later read" && f.remote.counts()["GET"] != 4 {
				t.Fatal("fault was not after first verified evidence")
			}
			t.Logf("JSON %s: stdout bytes=0; cleanup=1", kind)
		})
	}
}

func TestSearchJSONPinsSelectionAcrossTargetChange(t *testing.T) {
	f := setup(t)
	f.publish(t, makeBundle(t, "generation-a", "Retry uploads safely."))
	f.publish(t, makeBundle(t, "generation-b", "Retry requests carefully."))
	activateFixture(t, f, "generation-a")
	// Candidate B is strictly opened before SetTarget, just as activation requires.
	if _, err := publishedsearch.Open(testContext(t), f.cat, f.objects, publishedsearch.Selection{Collection: "guide", Generation: "generation-b", Edition: "one"}, testLimits()); err != nil {
		t.Fatal(err)
	}
	f.remote.reset()
	updateDone := make(chan error, 1)
	f.remote.mu.Lock()
	f.remote.onGET = func(n int) {
		if n == 3 {
			_, err := f.cat.SetTarget(testContext(t), catalog.TargetUpdate{Name: "kubernetes", ExpectedRevision: 1, ExpectedCollection: "guide", Value: catalog.TargetValue{Collection: "guide", Generation: "generation-b", DefaultEdition: "one"}})
			updateDone <- err
		}
	}
	f.remote.mu.Unlock()
	opened, closed := 0, 0
	var out bytes.Buffer
	if err := runSearch(testContext(t), []string{"--json", "--limit=1", "kubernetes", "retry"}, &out, environment(configEnvironment()), cliConnector(f, &opened, &closed)); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-updateDone:
		if err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatal("target update was not exercised")
	}
	got := decodeSearchJSON(t, out.Bytes())
	if got.Selection.Generation != "generation-a" || got.Results[0].Evidence.Quote != "Pods\nRetry uploads safely." {
		t.Fatal("mixed generation evidence", got)
	}
	f.remote.mu.Lock()
	f.remote.onGET = nil
	f.remote.mu.Unlock()
	out.Reset()
	if err := runSearch(testContext(t), []string{"--json", "--limit=1", "kubernetes", "retry"}, &out, environment(configEnvironment()), cliConnector(f, &opened, &closed)); err != nil {
		t.Fatal(err)
	}
	got = decodeSearchJSON(t, out.Bytes())
	if got.Selection.Generation != "generation-b" || got.Results[0].Evidence.Quote != "Pods\nRetry requests carefully." {
		t.Fatal("next request did not resolve new target", got)
	}
	t.Log("JSON target changed during first body GET: current response stays on A; next invocation selects B")
}

func TestSearchJSONRerankCountsAndNoExtraReads(t *testing.T) {
	f := setup(t)
	f.publish(t, makeBundleWithDocuments(t, "generation-a", "Retry uploads safely.", 102))
	activateFixture(t, f, "generation-a")
	env := configEnvironment()
	env["CONCORDANCE_WORKER_SOCKET"] = fixtureWorker(t, true)
	var out bytes.Buffer
	opened, closed := 0, 0
	if err := runSearch(testContext(t), []string{"--json", "--rerank", "--limit=1", "kubernetes", "why retry"}, &out, environment(env), cliConnector(f, &opened, &closed)); err != nil {
		t.Fatal(err)
	}
	got := decodeSearchJSON(t, out.Bytes())
	wantModel := searchJSONModel{rerank.Model, rerank.Revision, rerank.InputVersion, rerank.Runtime}
	if got.Retrieval.Policy != "any_terms_bm25_then_bge" || got.Retrieval.IndexedMatches != 102 || got.Retrieval.RankedCandidates != 100 || got.Retrieval.RankingVersion != "bge-v2-m3/lexical100/v1" || got.Retrieval.Model == nil || *got.Retrieval.Model != wantModel || len(got.Results) != 1 || got.Results[0].Source.NativeID != "docs/extra-99.md" || got.Results[0].Evidence.Quote != "Pods\nRetry uploads safely." || got.Results[0].Evidence.Headings == nil || len(got.Results[0].Evidence.Headings) != 0 || f.remote.counts()["GET"] != 3 || closed != 1 {
		t.Fatal("rerank count/evidence/read changed", got, f.remote.counts())
	}
	t.Log("JSON rerank: indexed=102 scored=100 shown=1 opening GETs=2 article GETs=1; no displayed-evidence reread")
	for _, query := range []string{"nonexistent", "retry"} {
		out.Reset()
		delete(env, "CONCORDANCE_WORKER_SOCKET")
		err := runSearch(testContext(t), []string{"--json", "--rerank", "kubernetes", query}, &out, environment(env), cliConnector(f, &opened, &closed))
		if query == "nonexistent" {
			if err != nil || decodeSearchJSON(t, out.Bytes()).Retrieval.RankedCandidates != 0 {
				t.Fatal("empty required worker", err)
			}
		} else if err == nil || out.Len() != 0 {
			t.Fatal("missing worker fell back", err)
		}
	}
}
