package generationpublish

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"testing"

	"github.com/sidsharma96/concordance/internal/catalog"
	gm "github.com/sidsharma96/concordance/internal/generationmanifest"
	"github.com/sidsharma96/concordance/internal/htmlarticle"
	"github.com/sidsharma96/concordance/internal/htmlarticlejson"
	"github.com/sidsharma96/concordance/internal/htmledition"
	"github.com/sidsharma96/concordance/internal/htmlpassage"
	"github.com/sidsharma96/concordance/internal/htmlterms"
	"github.com/sidsharma96/concordance/internal/textterms"
)

func capturedRules() CapturedRules {
	return CapturedRules{Input: gm.InputRules{ArticleProcessor: htmlarticle.Processor, PassageProcessor: htmlpassage.Processor, FieldProcessor: htmlterms.Processor, TermsVersion: textterms.Version, Profiles: []htmlarticle.Profile{{ID: "fixture/v1", ArticleClass: "fixture"}}}, MaxHashBytes: 1 << 20}
}
func capturedPublish(f storageFixture, b bundle, key string, r CapturedRules) (catalog.Publication, error) {
	return f.publisher.PublishCaptured(f.ctx, key, b.d.Generation, b.manifest, b.artifacts, r)
}
func TestCapturedPublicationRecoversSameInputs(t *testing.T) {
	f := setup(t)
	b := makeBundle(t)
	first, e := capturedPublish(f, b, "captured", capturedRules())
	if e != nil || first.Duplicate {
		t.Fatal(first, e)
	}
	record, e := f.cat.Lookup(f.ctx, "guide", "captured")
	if e != nil || record.Request.FingerprintVersion != gm.InputFingerprintVersion || record.Generation == nil {
		t.Fatal("derived request missing", e)
	}
	root := ref(b.manifest)
	m, e := gm.Open(f.ctx, b.manifest, root, "guide", b.d.Generation, testLimits().Manifest)
	if e != nil {
		t.Fatal(e)
	}
	digest, e := m.InputFingerprint(f.ctx, capturedRules().Input, 1<<20)
	if e != nil || digest != record.Request.Fingerprint {
		t.Fatal("saved digest does not match preflight inputs", e)
	}
	before := f.remote.counts()
	b.d.Generation = "new-candidate"
	b.encode(t)
	again, e := capturedPublish(f, b, "captured", capturedRules())
	if e != nil || !again.Duplicate || again.Generation != first.Generation || f.remote.counts() != before {
		t.Fatal("candidateID changed retry", again, e)
	}
	// Completed outcomes remain discoverable even if receipt trust is quarantined.
	if e = f.cat.Quarantine(f.ctx, f.ns); e != nil {
		t.Fatal(e)
	}
	again, e = capturedPublish(f, b, "captured", capturedRules())
	if e != nil || again.Generation != first.Generation || !again.Duplicate || f.remote.counts() != before {
		t.Fatal("completed retry after quarantine", e)
	}
	t.Log("same captured inputs + new candidate ID: original result, zero extra S3 requests; saved fingerprint version verified")
}

// Rebuild a valid checked bundle around either a changed captured representation
// or deliberately changed derived text. The latter models a producer/versioning
// bug; metadata checks cannot establish truthful extraction. It is never uploaded
// by the completed retry test and is not evidence of actual nondeterminism.
func rebuildCandidate(t *testing.T, b *bundle, changeHTML, changeDerived bool) {
	t.Helper()
	ctx := testContext(t)
	l := testLimits()
	d := &b.d.Editions[0]
	bodyFor := func(r gm.Ref) []byte {
		for _, a := range b.artifacts {
			if ref(a) == r {
				return a
			}
		}
		t.Fatal("missing fixture body")
		return nil
	}
	replace := func(old gm.Ref, next []byte) gm.Ref {
		for i, a := range b.artifacts {
			if ref(a) == old {
				b.artifacts[i] = bytes.Clone(next)
				return ref(next)
			}
		}
		t.Fatal("missing old body")
		return gm.Ref{}
	}
	oldHTML, oldArticle, oldIndex := d.Documents[0].HTML, d.Documents[0].Article, d.Index
	html := bytes.Clone(bodyFor(oldHTML))
	if changeHTML {
		html = append([]byte("<!-- changed retained capture -->\n"), html...)
	}
	a, e := htmlarticle.Extract(ctx, html, htmlarticle.Profile{ID: "fixture/v1", ArticleClass: "fixture"}, htmlarticle.Limits{MaxInputBytes: 1 << 20, MaxTokenBytes: 65536, MaxTokens: 10000, MaxNodes: 10000, MaxDepth: 64, MaxBlocks: 1000, MaxGroups: 1000, MaxBlockBytes: 65536, MaxOutputBytes: 1 << 20})
	if e != nil {
		t.Fatal(e)
	}
	if changeDerived {
		for i := range a.Blocks {
			if a.Blocks[i].Kind == htmlarticle.Prose {
				a.Blocks[i].Text += " Deliberately modelled producer drift."
				break
			}
		}
	}
	article, e := htmlarticlejson.Encode(ctx, a, l.Article)
	if e != nil {
		t.Fatal(e)
	}
	index, e := htmledition.Build(ctx, "one", [][]byte{article, article}, htmledition.BuildLimits{File: l.Edition, Article: l.Article, Preparation: prepLimits()})
	if e != nil {
		t.Fatal(e)
	}
	wire, e := htmledition.Encode(ctx, index, l.Edition)
	if e != nil {
		t.Fatal(e)
	}
	newHTML := replace(oldHTML, html)
	newArticle := replace(oldArticle, article)
	d.Index = replace(oldIndex, wire)
	for i := range d.Documents {
		d.Documents[i].HTML = newHTML
		d.Documents[i].Article = newArticle
	}
	b.d.Generation = "rebuilt-candidate"
	b.encode(t)
}
func TestCapturedPublicationConflictsBeforeObjectAccess(t *testing.T) {
	for _, name := range []string{"HTML", "same-ID policy", "scope", "new key"} {
		t.Run(name, func(t *testing.T) {
			f := setup(t)
			b := makeBundle(t)
			first, e := capturedPublish(f, b, "capture-key", capturedRules())
			if e != nil {
				t.Fatal(e)
			}
			before := f.remote.counts()
			r := capturedRules()
			key := "capture-key"
			switch name {
			case "HTML":
				rebuildCandidate(t, &b, true, false)
			case "same-ID policy":
				r.Input.Profiles[0].OmitClasses = []string{"unused-policy-token"}
			case "scope":
				b.d.Editions[0].Captures[0].Directory = "."
				b.d.Generation = "changed-scope"
				b.encode(t)
			case "new key":
				key = "deliberately-new-operation"
				b.d.Generation = "another-publication"
				b.encode(t)
			}
			out, e := capturedPublish(f, b, key, r)
			if name == "new key" {
				if e != nil || out.Duplicate || out.Generation.ID != "another-publication" {
					t.Fatal("new intent not allowed", out, e)
				}
				return
			}
			if !errors.Is(e, catalog.ErrConflict) || out != (catalog.Publication{}) || f.remote.counts() != before {
				t.Fatal("changed inputs did remote work", out, e)
			}
			original, e := f.cat.LookupGeneration(f.ctx, "guide", first.Generation.ID)
			if e != nil || original != first.Generation {
				t.Fatal("original result changed", e)
			}
			t.Log("changed captured inputs/rules: conflict before any extra S3 request")
		})
	}
}
func TestCapturedPublicationExcludesDerivedOutputBytes(t *testing.T) {
	f := setup(t)
	b := makeBundle(t)
	first, e := capturedPublish(f, b, "derived-drift", capturedRules())
	if e != nil {
		t.Fatal(e)
	}
	before := f.remote.counts()
	oldArticle := b.d.Editions[0].Documents[0].Article
	oldIndex := b.d.Editions[0].Index
	rebuildCandidate(t, &b, false, true)
	if b.d.Editions[0].Documents[0].Article == oldArticle || b.d.Editions[0].Index == oldIndex {
		t.Fatal("did not change derived bytes")
	}
	out, e := capturedPublish(f, b, "derived-drift", capturedRules())
	if e != nil || !out.Duplicate || out.Generation != first.Generation || f.remote.counts() != before {
		t.Fatal("derived bytes defined input identity", out, e)
	}
	t.Log("modelled derived-output drift under identical inputs/rules: original result, zero extra S3 requests")
}
func TestCapturedPublicationInvalidInputNeverBinds(t *testing.T) {
	for _, name := range []string{"missing profile", "unknown processor", "hash budget", "wrong body", "missing artifact", "invalid key"} {
		t.Run(name, func(t *testing.T) {
			f := setup(t)
			b := makeBundle(t)
			r := capturedRules()
			key := "invalid-captured"
			switch name {
			case "missing profile":
				r.Input.Profiles = nil
			case "unknown processor":
				r.Input.ArticleProcessor = "future"
			case "hash budget":
				r.MaxHashBytes = 8
			case "wrong body":
				b.artifacts[0] = []byte("wrong")
			case "missing artifact":
				b.artifacts = b.artifacts[1:]
			case "invalid key":
				key = ""
			}
			out, e := capturedPublish(f, b, key, r)
			if e == nil || out != (catalog.Publication{}) || f.remote.counts() != ([3]int{}) {
				t.Fatal("invalid input did remote work", out, e)
			}
			if key != "" {
				if _, e := f.cat.Lookup(f.ctx, "guide", key); !errors.Is(e, catalog.ErrNotFound) {
					t.Fatal("invalid input bound key", e)
				}
			}
		})
	}
}
func TestCapturedPublicationContextAndBusyAdmission(t *testing.T) {
	var p *Publisher
	if _, e := p.PublishCaptured(context.Background(), "k", "g", nil, nil, capturedRules()); !errors.Is(e, ErrInvalid) {
		t.Fatal(e)
	}
	f := setup(t)
	b := makeBundle(t)
	if _, e := f.publisher.PublishCaptured(nil, "k", b.d.Generation, b.manifest, b.artifacts, capturedRules()); !errors.Is(e, ErrInvalid) {
		t.Fatal(e)
	}
	if _, e := f.publisher.PublishCaptured(context.Background(), "k", b.d.Generation, b.manifest, b.artifacts, capturedRules()); !errors.Is(e, ErrInvalid) {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(f.ctx)
	cancel()
	if _, e := f.publisher.PublishCaptured(ctx, "k", b.d.Generation, b.manifest, b.artifacts, capturedRules()); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	f.publisher.permit <- struct{}{}
	_, e := capturedPublish(f, b, "busy", CapturedRules{})
	<-f.publisher.permit
	if !errors.Is(e, ErrBusy) {
		t.Fatal(e)
	}
	if f.remote.counts() != ([3]int{}) {
		t.Fatal("bad admission did IO")
	}
}
func TestCapturedRulesDoNotRetainCallerAliases(t *testing.T) {
	f := setup(t)
	b := makeBundle(t)
	r := capturedRules()
	r.Input.Profiles[0].OmitClasses = []string{"chrome", "footer"}
	before := sha256.Sum256(b.manifest)
	_, e := capturedPublish(f, b, "borrowed", r)
	if e != nil {
		t.Fatal(e)
	}
	if r.Input.Profiles[0].OmitClasses[0] != "chrome" || sha256.Sum256(b.manifest) != before {
		t.Fatal("caller data changed")
	}
	clear(r.Input.Profiles[0].OmitClasses)
	clear(b.manifest)
	for _, a := range b.artifacts {
		clear(a)
	}
	if _, e := f.cat.LookupGeneration(f.ctx, "guide", "generation-1"); e != nil {
		t.Fatal("clearing caller storage changed published metadata", e)
	}
}
