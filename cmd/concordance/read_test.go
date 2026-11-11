package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/sidsharma96/concordance/internal/catalog"
	"github.com/sidsharma96/concordance/internal/htmlpassage"
	"github.com/sidsharma96/concordance/internal/publishedsearch"
	"github.com/sidsharma96/concordance/internal/searchtarget"
)

func TestSourceReferenceCanonicalBoundary(t *testing.T) {
	ref := publishedsearch.SourceReference{Selection: publishedsearch.Selection{Collection: "guide", Generation: "g", Edition: "one"}, SourceID: "docs", NativeID: "docs/odd \" \u00e9.md", ArticleSHA256: [32]byte{1}, Body: htmlpassage.Range{First: 2, After: 3}}
	token, err := encodeSourceReference(ref)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeSourceReference(token)
	if err != nil || got != ref {
		t.Fatal(got, err)
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(token, sourceRefPrefix))
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{
		string(raw) + " ", strings.Replace(string(raw), `"slot":0`, `"slot":null`, 1), strings.Replace(string(raw), `"slot":0`, `"slot":0,"slot":0`, 1), strings.Replace(string(raw), `"slot":0`, `"slot":0,"unknown":1`, 1), strings.Replace(string(raw), `"slot":0`, `"slot":0.0`, 1), strings.Replace(string(raw), `"first":2`, `"first":-1`, 1), strings.Replace(string(raw), `"generation":"g"`, `"generation":"\ud800"`, 1),
	} {
		if _, err := decodeSourceReference(sourceRefPrefix + base64.RawURLEncoding.EncodeToString([]byte(s))); err == nil {
			t.Fatal("noncanonical accepted", s)
		}
	}
	for _, s := range []string{"", "cs2." + token[4:], token + "\n", strings.Repeat("x", maxSourceRefBytes+1)} {
		if _, err := decodeSourceReference(s); err == nil {
			t.Fatal("invalid token accepted")
		}
	}
}

func TestReadCommandKeepsOldGenerationAndNavigates(t *testing.T) {
	f := setup(t)
	b := makeMarkupBundle(t, "generation-a", "Example.", `<main class="fixture"><h1>Guide</h1><p>Intro.</p><h2>Rules</h2><div class="notice" title="Gate"><p>Alpha, disabled by default.</p></div><p>Example.</p><pre>restart: Never</pre><h2>Other</h2><p>Unrelated.</p></main>`, 2)
	f.publish(t, b)
	activateFixture(t, f, "generation-a")
	opened, closed := 0, 0
	connect := cliConnector(f, &opened, &closed)
	env := environment(configEnvironment())
	var out bytes.Buffer
	if err := runSearch(testContext(t), []string{"--json", "kubernetes", "example"}, &out, env, connect); err != nil {
		t.Fatal(err)
	}
	hit := decodeSearchJSON(t, out.Bytes()).Results[0]
	f.publish(t, makeBundle(t, "generation-b", "New source."))
	_, err := searchtarget.Activate(testContext(t), f.cat, f.objects, catalog.TargetUpdate{Name: "kubernetes", ExpectedRevision: 1, Value: catalog.TargetValue{Collection: "guide", Generation: "generation-b", DefaultEdition: "one"}}, testLimits())
	if err != nil {
		t.Fatal(err)
	}
	out.Reset()
	f.remote.reset()
	if err := runRead(testContext(t), []string{"--scope=section", hit.SourceRef}, &out, env, connect); err != nil {
		t.Fatal(err)
	}
	var got readJSON
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Selection.Generation != "generation-a" || got.Source != hit.Source || got.Evidence.FirstBlock != 2 || got.Evidence.AfterBlock != 6 || !strings.Contains(got.Evidence.Quote, "Alpha, disabled by default.") || !strings.Contains(got.Evidence.Quote, "restart: Never") || strings.Contains(got.Evidence.Quote, "Unrelated") || len(got.Notices) != 1 || got.Notices[0].Label != "Gate" || got.Navigation.Parent == nil || got.Navigation.Next == nil {
		t.Fatal(got)
	}
	if f.remote.counts()["GET"] != 3 || opened != closed {
		t.Fatal("read budget/lifetime", f.remote.counts(), opened, closed)
	}
	out.Reset()
	if err := runRead(testContext(t), []string{*got.Navigation.Next}, &out, env, connect); err != nil {
		t.Fatal(err)
	}
	var next readJSON
	json.Unmarshal(out.Bytes(), &next)
	if !strings.Contains(next.Evidence.Quote, "Unrelated") || next.Selection != got.Selection || next.Source != got.Source {
		t.Fatal(next)
	}
	out.Reset()
	if err := runRead(testContext(t), []string{"--max-read-bytes=1", got.SourceRef}, &out, env, connect); err == nil || out.Len() != 0 {
		t.Fatal("partial oversized read", err)
	}
	if err := runRead(testContext(t), []string{"--max-output-bytes=1", got.SourceRef}, &out, env, connect); !errors.Is(err, errOutputLimit) || out.Len() != 0 {
		t.Fatal("partial encoded read", err)
	}
	ctx, cancel := context.WithCancel(testContext(t))
	cancel()
	if err := runRead(ctx, []string{hit.SourceRef}, &out, env, connect); !errors.Is(err, context.Canceled) || out.Len() != 0 {
		t.Fatal("canceled read", err)
	}
	// Slot/hash/source/range mismatches may read metadata but never emit evidence.
	original, err := decodeSourceReference(hit.SourceRef)
	if err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*publishedsearch.SourceReference){
		func(r *publishedsearch.SourceReference) { r.ArticleSHA256[0] ^= 1 }, func(r *publishedsearch.SourceReference) { r.NativeID = "wrong" }, func(r *publishedsearch.SourceReference) { r.SourceID = "other" }, func(r *publishedsearch.SourceReference) { r.Slot = 900 }, func(r *publishedsearch.SourceReference) { r.Body.After = 999 }, func(r *publishedsearch.SourceReference) { r.Selection.Edition = "absent" }, func(r *publishedsearch.SourceReference) { r.Selection.Generation = "unpublished" },
	} {
		r := original
		mutate(&r)
		tok, err := encodeSourceReference(r)
		if err != nil {
			t.Fatal(err)
		}
		f.remote.reset()
		out.Reset()
		if err := runRead(testContext(t), []string{tok}, &out, env, connect); err == nil || out.Len() != 0 {
			t.Fatal("bad locator accepted", r, err)
		}
		if f.remote.counts()["GET"] > 2 {
			t.Fatal("invalid identity read body")
		}
	}
	// Corruption must fail strict reads, including after a prior successful read.
	f.remote.mu.Lock()
	path := f.path(t, ref(b.article))
	f.remote.bodies[path][0] ^= 1
	f.remote.mu.Unlock()
	out.Reset()
	if err := runRead(testContext(t), []string{hit.SourceRef}, &out, env, connect); err == nil || out.Len() != 0 {
		t.Fatal("corrupt source accepted", err)
	}
	if opened != closed {
		t.Fatal("leaked clients")
	}
}

func TestReadRejectsBeforeConnecting(t *testing.T) {
	calls := 0
	connect := func(context.Context, searchConfig, int64) (searchStores, error) {
		calls++
		return searchStores{}, errors.New("unexpected")
	}
	ref := publishedsearch.SourceReference{Selection: publishedsearch.Selection{Collection: "other", Generation: "g", Edition: "one"}, SourceID: "docs", NativeID: "a", Body: htmlpassage.Range{First: 0, After: 1}}
	token, err := encodeSourceReference(ref)
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{token}, {"bad"}, {"--scope=bad", token}, {"--max-read-bytes=0", token}} {
		var out bytes.Buffer
		if err := runRead(testContext(t), args, &out, environment(configEnvironment()), connect); err == nil || out.Len() != 0 {
			t.Fatal(args, err)
		}
	}
	if calls != 0 {
		t.Fatal(calls)
	}
	var out bytes.Buffer
	if err := runRead(testContext(t), []string{"--help"}, &out, environment(nil), connect); err != nil || !strings.Contains(out.String(), "SOURCE_REF") {
		t.Fatal(err)
	}
}

func TestSourceReferenceWorstCaseEscaping(t *testing.T) {
	ref := publishedsearch.SourceReference{
		Selection: publishedsearch.Selection{Collection: strings.Repeat("\x01", 256), Generation: strings.Repeat("&", 256), Edition: strings.Repeat("\x01", 8192)},
		SourceID:  strings.Repeat("<", 256), NativeID: strings.Repeat("\x01", 8192), Slot: int(^uint(0) >> 1), ArticleSHA256: [32]byte{1},
		Body: htmlpassage.Range{First: 0, After: int(^uint(0) >> 1)},
	}
	token, err := encodeSourceReference(ref)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeSourceReference(token)
	if err != nil || got != ref {
		t.Fatal("admitted fields exceeded transport budget", err)
	}
}
