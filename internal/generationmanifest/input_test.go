package generationmanifest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/sidsharma96/concordance/internal/htmlarticle"
	"github.com/sidsharma96/concordance/internal/htmlpassage"
	"github.com/sidsharma96/concordance/internal/htmlterms"
	"github.com/sidsharma96/concordance/internal/textterms"
)

func inputRules() InputRules {
	return InputRules{ArticleProcessor: htmlarticle.Processor, PassageProcessor: htmlpassage.Processor, FieldProcessor: htmlterms.Processor, TermsVersion: textterms.Version, Profiles: []htmlarticle.Profile{{ID: "fixture/v1", ArticleClass: "fixture", OmitClasses: []string{"chrome", "footer"}, OmitIDs: []string{"feedback", "tools"}, NoticeClasses: []string{"notice", "alert"}}}}
}
func openDescription(t testing.TB, d Description) *Manifest {
	t.Helper()
	ctx := testContext(t)
	b, e := Encode(ctx, d, testLimits())
	if e != nil {
		t.Fatal(e)
	}
	m, e := Open(ctx, b, reference(b), d.Collection, d.Generation, testLimits())
	if e != nil {
		t.Fatal(e)
	}
	return m
}
func fingerprint(t testing.TB, d Description, r InputRules) [32]byte {
	t.Helper()
	h, e := openDescription(t, d).InputFingerprint(testContext(t), r, 1<<20)
	if e != nil {
		t.Fatal(e)
	}
	return h
}
func TestInputFingerprintFramingVector(t *testing.T) {
	// Independent preimage: uint64 big-endian string lengths + UTF-8 strings;
	// domain, collection, retained formats, four versions, profiles0, sources0,
	// editions1, "empty", captures0, documents0. No production writer used by oracle.
	d := Description{Collection: "guide", Generation: "candidate", Editions: []Edition{{ID: "empty", Index: reference([]byte("empty-index"))}}}
	r := inputRules()
	r.Profiles = nil
	m := openDescription(t, d)
	h, e := m.InputFingerprint(testContext(t), r, 1000)
	if e != nil {
		t.Fatal(e)
	}
	const expected = "5a4bbf96f1e79c34eb3b3bf7812678ac152a388ebd6c76059df4ce6b93e3eb04"
	if hex.EncodeToString(h[:]) != expected {
		t.Fatalf("framing drift: %x", h)
	}
	const size = 267
	if got, e := m.InputFingerprint(testContext(t), r, size); e != nil || got != h {
		t.Fatal("exact budget", e)
	}
	if got, e := m.InputFingerprint(testContext(t), r, size-1); !errors.Is(e, ErrLimit) || got != ([32]byte{}) {
		t.Fatal("one below", got, e)
	}
	t.Logf("empty-edition input vector: framed_bytes=%d; exact budget passes; one byte less rejected", size)
}
func TestInputFingerprintSensitivity(t *testing.T) {
	base := fixture(t).d
	want := fingerprint(t, base, inputRules())
	changes := []struct {
		name      string
		change    func(*Description, *InputRules)
		different bool
	}{
		{"generation", func(d *Description, r *InputRules) { d.Generation = "another-candidate" }, false},
		{"article output", func(d *Description, r *InputRules) {
			d.Editions[0].Documents[0].Article = reference([]byte("another-article"))
		}, false},
		{"index output", func(d *Description, r *InputRules) { d.Editions[0].Index = reference([]byte("another-index")) }, false},
		{"image block output", func(d *Description, r *InputRules) { d.Editions[0].Documents[0].Images[0].Block++ }, false},
		{"collection", func(d *Description, r *InputRules) { d.Collection = "another-collection" }, true},
		{"repository", func(d *Description, r *InputRules) { d.Sources[0].Repository = "https://example.invalid/other.git" }, true},
		{"source ID", func(d *Description, r *InputRules) {
			d.Sources[0].ID = "renamed"
			d.Editions[0].Captures[0].Source = "renamed"
		}, true},
		{"revision", func(d *Description, r *InputRules) { d.Editions[0].Captures[0].Revision = strings.Repeat("b", 40) }, true},
		{"scope", func(d *Description, r *InputRules) { d.Editions[0].Captures[0].Directory = "." }, true},
		{"preparation", func(d *Description, r *InputRules) { d.Editions[0].Captures[0].Preparation = "another-preparation/v1" }, true},
		{"toolchain", func(d *Description, r *InputRules) { d.Editions[0].Captures[0].Toolchain = "another-toolchain/v1" }, true},
		{"recipe", func(d *Description, r *InputRules) {
			d.Editions[0].Captures[0].Recipe = reference([]byte("different recipe"))
		}, true},
		{"raw body", func(d *Description, r *InputRules) {
			d.Editions[0].Captures[0].Files[0].Object = reference([]byte("different raw bytes"))
		}, true},
		{"Git blob", func(d *Description, r *InputRules) {
			d.Editions[0].Captures[0].Files[0].BlobID = strings.Repeat("c", 40)
		}, true},
		{"file inventory", func(d *Description, r *InputRules) {
			c := &d.Editions[0].Captures[0]
			c.Files = append(c.Files, File{Path: "data/dependency", BlobID: strings.Repeat("d", 40), Object: reference([]byte("dependency"))})
		}, true},
		{"logical path", func(d *Description, r *InputRules) {
			c := &d.Editions[0].Captures[0]
			c.Files[0].Path = "docs/renamed.md"
			doc := &d.Editions[0].Documents[0]
			doc.NativeID = c.Files[0].Path
			doc.Location = c.Files[0].Path
		}, true},
		{"HTML capture", func(d *Description, r *InputRules) {
			d.Editions[0].Documents[0].HTML = reference([]byte("different captured HTML"))
		}, true},
		{"document slot order", func(d *Description, r *InputRules) { slices.Reverse(d.Editions[0].Documents) }, true},
		{"edition identity", func(d *Description, r *InputRules) { d.Editions[0].ID = "two" }, true},
		{"empty edition absent", func(d *Description, r *InputRules) { d.Editions = d.Editions[:1] }, true},
		{"full same-ID profile policy", func(d *Description, r *InputRules) {
			r.Profiles[0].OmitClasses = append(r.Profiles[0].OmitClasses, "extra")
		}, true},
		{"article selector", func(d *Description, r *InputRules) { r.Profiles[0].ArticleClass = "another-root" }, true},
		{"omit IDs", func(d *Description, r *InputRules) { r.Profiles[0].OmitIDs = append(r.Profiles[0].OmitIDs, "extra") }, true},
		{"notice classes", func(d *Description, r *InputRules) {
			r.Profiles[0].NoticeClasses = append(r.Profiles[0].NoticeClasses, "extra")
		}, true},
		{"profile name", func(d *Description, r *InputRules) {
			r.Profiles[0].ID = "fixture/v2"
			for i := range d.Editions[0].Documents {
				d.Editions[0].Documents[i].Profile = "fixture/v2"
			}
		}, true},
	}
	for _, tc := range changes {
		t.Run(tc.name, func(t *testing.T) {
			d, e := openDescription(t, base).Describe(testContext(t))
			if e != nil {
				t.Fatal(e)
			}
			r := inputRules()
			tc.change(&d, &r)
			h := fingerprint(t, d, r)
			if (h != want) != tc.different {
				t.Fatal("unexpected input equality")
			}
			if tc.name == "generation" || tc.name == "HTML capture" || tc.name == "article output" || tc.name == "index output" {
				t.Logf("%s: same_input=%t", tc.name, h == want)
			}
		})
	}
}
func TestInputFingerprintNormalizesUnorderedRecords(t *testing.T) {
	d := fixture(t).d
	// Additional independent source/capture makes ordinal remapping observable.
	d.Sources = append(d.Sources, Source{ID: "another", Repository: "https://example.invalid/another.git"})
	c := d.Editions[0].Captures[0]
	c.Source = "another"
	c.Files = slices.Clone(c.Files)
	d.Editions[0].Captures = append(d.Editions[0].Captures, c)
	doc := d.Editions[0].Documents[0]
	doc.Capture = 1
	doc.Images = slices.Clone(doc.Images)
	d.Editions[0].Documents = append(d.Editions[0].Documents, doc)
	r := inputRules()
	r.Profiles = append(r.Profiles, htmlarticle.Profile{ID: "second/v1", ArticleClass: "other"})
	d.Editions[0].Documents[1].Profile = "second/v1"
	m := openDescription(t, d)
	before, e := m.Describe(testContext(t))
	if e != nil {
		t.Fatal(e)
	}
	want, err := m.InputFingerprint(testContext(t), r, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	slices.Reverse(d.Sources)
	slices.Reverse(d.Editions)
	e0 := &d.Editions[1]
	slices.Reverse(e0.Captures)
	for i := range e0.Documents {
		e0.Documents[i].Capture = 1 - e0.Documents[i].Capture
	}
	for ci := range e0.Captures {
		fs := e0.Captures[ci].Files
		slices.Reverse(fs)
		for i := range e0.Documents {
			doc := &e0.Documents[i]
			if doc.Capture != ci {
				continue
			}
			doc.File = len(fs) - 1 - doc.File
			for j := range doc.Images {
				doc.Images[j].File = len(fs) - 1 - doc.Images[j].File
			}
		}
	}
	slices.Reverse(r.Profiles)
	for i := range r.Profiles {
		slices.Reverse(r.Profiles[i].OmitClasses)
		slices.Reverse(r.Profiles[i].OmitIDs)
		slices.Reverse(r.Profiles[i].NoticeClasses)
	}
	if got := fingerprint(t, d, r); got != want {
		t.Fatal("unordered metadata changed identity")
	}
	after, e := m.Describe(testContext(t))
	if e != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("fingerprinting mutated immutable metadata", e)
	}
}
func TestInputFingerprintRejectsInvalidRulesAndContexts(t *testing.T) {
	m := openDescription(t, fixture(t).d)
	changes := []func(*InputRules){
		func(r *InputRules) { r.Profiles = nil }, func(r *InputRules) { r.Profiles = append(r.Profiles, r.Profiles[0]) },
		func(r *InputRules) {
			r.Profiles = append(r.Profiles, htmlarticle.Profile{ID: "unused", ArticleClass: "root"})
		},
		func(r *InputRules) { r.Profiles[0].ArticleClass = "bad class" }, func(r *InputRules) { r.Profiles[0].OmitClasses = []string{"fixture"} },
		func(r *InputRules) { r.Profiles[0].OmitIDs = []string{"dup", "dup"} }, func(r *InputRules) { r.Profiles[0].NoticeClasses = []string{"bad/notice"} },
		func(r *InputRules) { r.Profiles[0].OmitClasses = make([]string, 33) },
	}
	for i, change := range changes {
		r := inputRules()
		change(&r)
		if h, e := m.InputFingerprint(testContext(t), r, 1<<20); !errors.Is(e, ErrInvalid) || h != ([32]byte{}) {
			t.Fatal("invalid rules", i, h, e)
		}
	}
	for _, change := range []func(*InputRules){func(r *InputRules) { r.ArticleProcessor = "future" }, func(r *InputRules) { r.PassageProcessor = "future" }, func(r *InputRules) { r.FieldProcessor = "future" }, func(r *InputRules) { r.TermsVersion = "future" }} {
		r := inputRules()
		change(&r)
		if h, e := m.InputFingerprint(testContext(t), r, 1<<20); !errors.Is(e, ErrUnsupported) || h != ([32]byte{}) {
			t.Fatal("unsupported pipeline", e)
		}
	}
	for _, ctx := range []context.Context{nil, context.Background()} {
		if h, e := m.InputFingerprint(ctx, inputRules(), 1<<20); !errors.Is(e, ErrInvalid) || h != ([32]byte{}) {
			t.Fatal("invalid context", e)
		}
	}
	ctx, cancel := context.WithCancel(testContext(t))
	cancel()
	if h, e := m.InputFingerprint(ctx, inputRules(), 1<<20); !errors.Is(e, context.Canceled) || h != ([32]byte{}) {
		t.Fatal("cancelled", e)
	}
	for _, n := range []int{0, -1} {
		if h, e := m.InputFingerprint(testContext(t), inputRules(), n); !errors.Is(e, ErrInvalid) || h != ([32]byte{}) {
			t.Fatal("invalid limit", e)
		}
	}
	for _, bad := range []*Manifest{nil, {}} {
		if h, e := bad.InputFingerprint(testContext(t), inputRules(), 1<<20); !errors.Is(e, ErrInvalid) || h != ([32]byte{}) {
			t.Fatal("invalid state", e)
		}
	}
	r := inputRules()
	r.Profiles[0].OmitClasses = []string{strings.Repeat("x", 10000)}
	if h, e := m.InputFingerprint(testContext(t), r, 128); !errors.Is(e, ErrLimit) || h != ([32]byte{}) {
		t.Fatal("admit bytes before scratch", e)
	}
}
func TestInputFingerprintOwnershipAndConcurrentReads(t *testing.T) {
	m := openDescription(t, fixture(t).d)
	r := inputRules()
	before := slices.Clone(r.Profiles[0].OmitClasses)
	want, e := m.InputFingerprint(testContext(t), r, 1<<20)
	if e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(r.Profiles[0].OmitClasses, before) {
		t.Fatal("rules mutated")
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			h, e := m.InputFingerprint(testContext(t), r, 1<<20)
			if e != nil || h != want {
				t.Error("concurrent fingerprint", e)
			}
		}()
	}
	wg.Wait()
	r.Profiles[0].OmitClasses[0] = "changed-after-return"
	if h, e := m.InputFingerprint(testContext(t), r, 1<<20); e != nil || h == want {
		t.Fatal("caller changes lost", e)
	}
	if want == sha256.Sum256(nil) {
		t.Fatal("unexpected digest")
	}
}

// Cancel deterministically during traversal, without a racing mutator or sleeps.
type inputCancelContext struct {
	context.Context
	checks, left int
}

func (c *inputCancelContext) Err() error {
	c.checks++
	if c.checks >= c.left {
		return context.Canceled
	}
	return c.Context.Err()
}
func TestInputFingerprintCancellationDuringTraversal(t *testing.T) {
	m := openDescription(t, fixture(t).d)
	probe := &inputCancelContext{Context: testContext(t), left: int(^uint(0) >> 1)}
	if _, e := m.InputFingerprint(probe, inputRules(), 1<<20); e != nil {
		t.Fatal(e)
	}
	for _, after := range []int{10, probe.checks / 2, probe.checks - 1} {
		ctx := &inputCancelContext{Context: testContext(t), left: after}
		h, e := m.InputFingerprint(ctx, inputRules(), 1<<20)
		if !errors.Is(e, context.Canceled) || h != ([32]byte{}) {
			t.Fatal("partial fingerprint escaped", after, h, e)
		}
	}
}

func TestInputFingerprintRichVector(t *testing.T) {
	// Independently framed source/recipe/file/document/profile records from the
	// published v1 specification; expected digest is not produced by InputFingerprint.
	m := openDescription(t, fixture(t).d)
	const size = 1273
	const expected = "32da98d19ab242286d6e862a1242fea06f60422b42fc93c6ba780fcda697834f"
	h, e := m.InputFingerprint(testContext(t), inputRules(), size)
	if e != nil || hex.EncodeToString(h[:]) != expected {
		t.Fatalf("rich framing drift: %x %v", h, e)
	}
	if h, e := m.InputFingerprint(testContext(t), inputRules(), size-1); !errors.Is(e, ErrLimit) || h != ([32]byte{}) {
		t.Fatal("rich budget", h, e)
	}
}
