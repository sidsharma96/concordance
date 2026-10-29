package generationmanifest

import (
	"bytes"
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sidsharma96/concordance/internal/htmlarticle"
	"github.com/sidsharma96/concordance/internal/htmlarticlejson"
	"github.com/sidsharma96/concordance/internal/htmledition"
	"github.com/sidsharma96/concordance/internal/htmlpassage"
	"github.com/sidsharma96/concordance/internal/htmlterms"
	"github.com/sidsharma96/concordance/internal/termsearch"
	"github.com/sidsharma96/concordance/internal/textterms"
)

func testContext(t testing.TB) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	return ctx
}
func testLimits() Limits {
	return Limits{MaxEncodedBytes: 1 << 20, MaxSources: 8, MaxEditions: 8, MaxCaptures: 16, MaxFiles: 128, MaxDocuments: 64, MaxImages: 128, MaxSourceFiles: 128, MaxSourceDocuments: 64, MaxStringBytes: 4096, MaxValueBytes: 1 << 18, MaxObjectBytes: 1 << 20, MaxReferencedBytes: 16 << 20}
}
func articleLimits() htmlarticlejson.Limits {
	return htmlarticlejson.Limits{MaxEncodedBytes: 1 << 20, MaxBlocks: 1000, MaxGroups: 1000, MaxStringBytes: 65536, MaxValueBytes: 1 << 20, MaxGroupDepth: 64, MaxHTMLBytes: 1 << 20}
}
func indexLimits() htmledition.Limits {
	return htmledition.Limits{MaxEncodedBytes: 1 << 20, MaxArticles: 64, MaxArticleBytes: 1 << 20, MaxBlocks: 10000, MaxPassages: 1000, MaxTerms: 20000, MaxPreparedBytes: 1 << 20, MaxKeyBytes: 1 << 20, MaxTermBytes: 1024, MaxVocabulary: 10000, MaxPostings: 20000}
}
func preparationLimits() htmlterms.Limits {
	return htmlterms.Limits{Passages: htmlpassage.Limits{MaxBlocks: 1000, MaxGroups: 1000, MaxInputBytes: 1 << 20, MaxPassages: 1000, MaxBodyBytes: 4096, MaxReferences: 10000, MaxContextBytes: 1 << 20}, Field: textterms.Limits{MaxInputBytes: 65536, MaxTerms: 10000, MaxTermBytes: 1024, MaxOutputBytes: 1 << 20}, MaxViewBytes: 1 << 20, MaxTerms: 20000, MaxOutputBytes: 1 << 20}
}
func reference(b []byte) Ref { return Ref{sha256.Sum256(b), int64(len(b))} }
func gitBlob(b []byte) string {
	h := sha1.New()
	fmt.Fprintf(h, "blob %d\x00", len(b))
	h.Write(b)
	return hex.EncodeToString(h.Sum(nil))
}

type fixtureData struct {
	d       Description
	objects map[Ref][]byte
	article []byte
	index   []byte
	quote   string
}

// Controlled synthetic producer, not a production Git/preparation adapter. Two
// logical Markdown paths deliberately produce identical retained article bytes.
func fixture(t testing.TB) fixtureData {
	t.Helper()
	ctx := testContext(t)
	f := fixtureData{objects: make(map[Ref][]byte)}
	put := func(b []byte) Ref { r := reference(b); f.objects[r] = bytes.Clone(b); return r }
	html := []byte(`<main class="fixture"><h1>Pods</h1><p>Retry uploads safely.</p><figure><img src="/images/pod.svg" alt="Pod diagram"><figcaption>Pod diagram</figcaption></figure></main>`)
	a, err := htmlarticle.Extract(ctx, html, htmlarticle.Profile{ID: "fixture/v1", ArticleClass: "fixture"}, htmlarticle.Limits{MaxInputBytes: 1 << 20, MaxTokenBytes: 65536, MaxTokens: 10000, MaxNodes: 10000, MaxDepth: 64, MaxBlocks: 1000, MaxGroups: 1000, MaxBlockBytes: 65536, MaxOutputBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	f.article, err = htmlarticlejson.Encode(ctx, a, articleLimits())
	if err != nil {
		t.Fatal(err)
	}
	idx, err := htmledition.Build(ctx, "one", [][]byte{f.article, f.article}, htmledition.BuildLimits{File: indexLimits(), Article: articleLimits(), Preparation: preparationLimits()})
	if err != nil {
		t.Fatal(err)
	}
	f.index, err = htmledition.Encode(ctx, idx, indexLimits())
	if err != nil {
		t.Fatal(err)
	}
	empty, err := htmledition.Build(ctx, "empty", nil, htmledition.BuildLimits{File: indexLimits(), Article: articleLimits(), Preparation: preparationLimits()})
	if err != nil {
		t.Fatal(err)
	}
	emptyBytes, err := htmledition.Encode(ctx, empty, indexLimits())
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte("# Pods\n\nRetry uploads safely.\n")
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><text>Pod</text></svg>`)
	c := Capture{Source: "docs", Revision: strings.Repeat("a", 40), Directory: "docs", Preparation: "fixture-preparation/v1", Toolchain: "fixture-builder/v1", Recipe: put([]byte(`{"tool":"fixture-builder/v1","synthetic":true}`)), Files: []File{{"docs/pods.md", gitBlob(raw), put(raw)}, {"docs/other.md", gitBlob(raw), put(raw)}, {"static/images/pod.svg", gitBlob(svg), put(svg)}}}
	var images []Image
	for i, b := range a.Blocks {
		if b.Kind == htmlarticle.Image {
			images = append(images, Image{Block: i, Address: b.SourceURL, File: 2})
		}
	}
	if len(images) != 1 {
		t.Fatalf("expected one image, got %d", len(images))
	}
	doc := Document{Capture: 0, File: 0, NativeID: c.Files[0].Path, Location: c.Files[0].Path, Profile: a.Profile, HTML: put(html), Article: put(f.article), Images: images}
	other := doc
	other.File = 1
	other.NativeID = c.Files[1].Path
	other.Location = c.Files[1].Path
	other.Images = append([]Image{}, images...)
	f.d = Description{Collection: "fixture-collection", Generation: "generation-1", Sources: []Source{{"docs", "https://example.invalid/docs.git"}}, Editions: []Edition{{ID: "one", Index: put(f.index), Captures: []Capture{c}, Documents: []Document{doc, other}}, {ID: "empty", Index: put(emptyBytes)}}}
	return f
}
func encoded(t testing.TB, d Description) []byte {
	t.Helper()
	b, err := Encode(testContext(t), d, testLimits())
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func opened(t testing.TB, b []byte) *Manifest {
	t.Helper()
	m, err := Open(testContext(t), b, reference(b), "fixture-collection", "generation-1", testLimits())
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestRoundTripArtifactsAndOwnership(t *testing.T) {
	f := fixture(t)
	body := encoded(t, f.d)
	m := opened(t, body)
	ctx := testContext(t)
	first, err := m.Describe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	again, err := Encode(ctx, first, testLimits())
	if err != nil || !bytes.Equal(body, again) {
		t.Fatalf("canonical roundtrip: %v", err)
	}
	refs, err := m.Objects(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != len(f.objects) {
		t.Fatalf("refs %d, objects %d", len(refs), len(f.objects))
	}
	t.Logf("synthetic manifest: %d encoded bytes, %d distinct referenced objects, 2 logical article slots, 2 editions", len(body), len(refs))
	for _, r := range refs {
		if _, ok := f.objects[r]; !ok {
			t.Fatal("unexpected object")
		}
	}
	x, err := m.OpenEdition(ctx, "one", f.index, indexLimits())
	if err != nil {
		t.Fatal(err)
	}
	if x.ArticleCount() != 2 {
		t.Fatal("logical slots collapsed")
	}
	q, err := textterms.Prepare(ctx, "retry", textterms.Limits{MaxInputBytes: 256, MaxTerms: 16, MaxTermBytes: 256, MaxOutputBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	hits, err := x.Search(ctx, termsearch.Query{TermsVersion: q.Version, Edition: "one", Terms: q.Terms}, termsearch.Limits{MaxEditions: 1, MaxPassages: 1000, MaxTerms: 20000, MaxQueryTerms: 16, MaxInputBytes: 1 << 20, MaxMatches: 1000})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits.Matches) != 2 {
		t.Fatalf("hits: %+v", hits)
	}
	for _, hit := range hits.Matches {
		loc, err := x.Location(hit.Passage)
		if err != nil {
			t.Fatal(err)
		}
		a, err := m.VerifyArticle(ctx, "one", loc.Article, f.article, articleLimits())
		if err != nil {
			t.Fatal(err)
		}
		quote, err := x.Quote(ctx, hit.Passage, f.article, articleLimits(), 4096)
		if err != nil {
			t.Fatal(err)
		}
		var parts []string
		for _, b := range a.Blocks[loc.Body.First:loc.Body.After] {
			parts = append(parts, b.Text)
		}
		if quote != strings.Join(parts, "\n") {
			t.Fatal("quote differs from independently joined retained blocks")
		}
	}
	if _, err := m.OpenEdition(ctx, "missing", nil, indexLimits()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
	if _, err := m.OpenEdition(ctx, "empty", f.objects[f.d.Editions[1].Index], indexLimits()); err != nil {
		t.Fatal(err)
	}
	// Clear encoded input and mutate all exported aliases. Private state survives.
	clear(body)
	f.d.Editions[0].Documents[0].NativeID = "changed"
	first.Sources[0].ID = "changed"
	first.Editions[0].Captures[0].Files[0].Path = "changed"
	first.Editions[0].Documents[0].Images[0].Address = "changed"
	refs[0] = Ref{}
	second, err := m.Describe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if second.Sources[0].ID != "docs" || second.Editions[0].Captures[0].Files[0].Path != "docs/pods.md" || second.Editions[0].Documents[0].Images[0].Address != "/images/pod.svg" {
		t.Fatal("mutable alias escaped")
	}
	if _, err := m.VerifyArticle(ctx, "one", 0, f.article, articleLimits()); err != nil {
		t.Fatal(err)
	}
	binding, err := m.Resolve(ctx, "one", 0)
	if err != nil {
		t.Fatal(err)
	}
	if binding.Source.ID != "docs" || binding.NativeID != "docs/pods.md" || binding.Revision != strings.Repeat("a", 40) || binding.Images[0].Input.Path != "static/images/pod.svg" {
		t.Fatal("incorrect resolved provenance")
	}
	binding.Images[0].Input.Path = "changed"
	binding.Source.ID = "changed"
	next, err := m.Resolve(ctx, "one", 0)
	if err != nil || next.Source.ID != "docs" || next.Images[0].Input.Path != "static/images/pod.svg" {
		t.Fatal("binding alias escaped", err)
	}
}

func TestStrictWire(t *testing.T) {
	f := fixture(t)
	base := string(encoded(t, f.d))
	ctx := testContext(t)
	cases := map[string]string{
		"duplicate":           strings.Replace(base, `"collection":"fixture-collection"`, `"collection":"fixture-collection","collection":"fixture-collection"`, 1),
		"escaped-duplicate":   strings.Replace(base, `"collection":"fixture-collection"`, `"collection":"fixture-collection","collect\u0069on":"fixture-collection"`, 1),
		"casefold":            strings.Replace(base, `"collection":`, `"Collection":`, 1),
		"unknown":             strings.Replace(base, `"collection":`, `"extra":0,"collection":`, 1),
		"missing":             strings.Replace(base, `,"collection":"fixture-collection"`, "", 1),
		"null":                strings.Replace(base, `"sources":[`, `"sources":null,"unused":[`, 1),
		"null-reference":      strings.Replace(base, `"html":{`, `"html":null,"unused":{`, 1),
		"fraction":            strings.Replace(base, `"capture":0`, `"capture":0.0`, 1),
		"exponent":            strings.Replace(base, `"capture":0`, `"capture":0e0`, 1),
		"negative-zero":       strings.Replace(base, `"capture":0`, `"capture":-0`, 1),
		"negative":            strings.Replace(base, `"capture":0`, `"capture":-1`, 1),
		"string-number":       strings.Replace(base, `"capture":0`, `"capture":"0"`, 1),
		"overflow":            strings.Replace(base, `"capture":0`, `"capture":9223372036854775808`, 1),
		"nested-case":         strings.Replace(base, `"blob_id":`, `"Blob_ID":`, 1),
		"uppercase-digest":    strings.Replace(base, `"sha256":"`, `"sha256":"A`, 1),
		"surrogate":           strings.Replace(base, `https://example.invalid/docs.git`, `\ud800`, 1),
		"low-surrogate":       strings.Replace(base, `https://example.invalid/docs.git`, `\udc00`, 1),
		"nul":                 strings.Replace(base, `https://example.invalid/docs.git`, `\u0000`, 1),
		"utf8":                strings.Replace(base, `https://example.invalid/docs.git`, string([]byte{0xff}), 1),
		"trailing":            base + ` {}`,
		"unsupported-format":  strings.Replace(base, Format, Format+"-new", 1),
		"unsupported-index":   strings.Replace(base, indexFormat, indexFormat+"-new", 1),
		"unsupported-article": strings.Replace(base, articleFormat, articleFormat+"-new", 1),
	}
	for name, s := range cases {
		t.Run(name, func(t *testing.T) {
			b := []byte(s)
			m, err := Open(ctx, b, reference(b), f.d.Collection, f.d.Generation, testLimits())
			if err == nil || m != nil {
				t.Fatalf("accepted hostile %s", name)
			}
		})
	}
	for n := 0; n < len(base); n++ {
		b := []byte(base[:n])
		m, err := Open(ctx, b, reference(b), f.d.Collection, f.d.Generation, testLimits())
		if err == nil || m != nil {
			t.Fatalf("accepted truncation %d", n)
		}
	}
	for _, replacement := range []string{`\ud834\udd1e`, "\ufffd"} {
		b := []byte(strings.Replace(base, `https://example.invalid/docs.git`, replacement, 1))
		if _, err := Open(ctx, b, reference(b), f.d.Collection, f.d.Generation, testLimits()); err != nil {
			t.Fatalf("valid Unicode: %v", err)
		}
	}
	// JSON member order and whitespace do not change decoded associations.
	var v any
	if err := json.Unmarshal([]byte(base), &v); err != nil {
		t.Fatal(err)
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Open(ctx, b, reference(b), f.d.Collection, f.d.Generation, testLimits()); err != nil {
		t.Fatal(err)
	}
}

func TestMetadataFailures(t *testing.T) {
	mutations := map[string]func(*Description){
		"duplicate-source":  func(d *Description) { d.Sources = append(d.Sources, d.Sources[0]) },
		"duplicate-edition": func(d *Description) { d.Editions = append(d.Editions, d.Editions[0]) },
		"undeclared-source": func(d *Description) { d.Editions[0].Captures[0].Source = "missing" },
		"duplicate-capture": func(d *Description) {
			d.Editions[0].Captures = append(d.Editions[0].Captures, d.Editions[0].Captures[0])
		},
		"bad-revision":   func(d *Description) { d.Editions[0].Captures[0].Revision = "main" },
		"bad-blob-width": func(d *Description) { d.Editions[0].Captures[0].Files[0].BlobID = strings.Repeat("a", 64) },
		"duplicate-file": func(d *Description) {
			d.Editions[0].Captures[0].Files = append(d.Editions[0].Captures[0].Files, d.Editions[0].Captures[0].Files[0])
		},
		"bad-path":        func(d *Description) { d.Editions[0].Captures[0].Files[0].Path = "../pods.md" },
		"capture-range":   func(d *Description) { d.Editions[0].Documents[0].Capture = 1 },
		"file-range":      func(d *Description) { d.Editions[0].Documents[0].File = 3 },
		"native-location": func(d *Description) { d.Editions[0].Documents[0].NativeID = "other" },
		"location":        func(d *Description) { d.Editions[0].Documents[0].Location = "other" },
		"out-of-scope":    func(d *Description) { d.Editions[0].Captures[0].Directory = "other" },
		"duplicate-key":   func(d *Description) { d.Editions[0].Documents[1] = d.Editions[0].Documents[0] },
		"size-conflict":   func(d *Description) { d.Editions[0].Documents[1].Article.Bytes++ },
		"empty-index":     func(d *Description) { d.Editions[0].Index.Bytes = 0 },
		"negative-ref":    func(d *Description) { d.Editions[0].Captures[0].Recipe.Bytes = -1 },
		"image-file":      func(d *Description) { d.Editions[0].Documents[0].Images[0].File = 0 },
		"image-address":   func(d *Description) { d.Editions[0].Documents[0].Images[0].Address = "https://example.invalid/pod.svg" },
		"image-traversal": func(d *Description) { d.Editions[0].Documents[0].Images[0].Address = "/images/../pod.svg" },
		"duplicate-image-block": func(d *Description) {
			doc := &d.Editions[0].Documents[0]
			doc.Images = append(doc.Images, doc.Images[0])
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			f := fixture(t)
			mutate(&f.d)
			body, err := Encode(testContext(t), f.d, testLimits())
			if err == nil || body != nil {
				t.Fatal("accepted invalid description")
			}
		})
	}
}

func TestArtifactMismatches(t *testing.T) {
	f := fixture(t)
	ctx := testContext(t)
	m := opened(t, encoded(t, f.d))
	bad := bytes.Clone(f.index)
	bad[0] ^= 1
	if x, err := m.OpenEdition(ctx, "one", bad, indexLimits()); err == nil || x != nil {
		t.Fatal("accepted bad index")
	}
	bad = bytes.Clone(f.article)
	bad[0] ^= 1
	if _, err := m.VerifyArticle(ctx, "one", 0, bad, articleLimits()); err == nil {
		t.Fatal("accepted bad article")
	}
	for _, slot := range []int{-1, 2} {
		if _, err := m.VerifyArticle(ctx, "one", slot, f.article, articleLimits()); !errors.Is(err, ErrInvalid) {
			t.Fatal(err)
		}
	}
	for _, change := range []func(*Description){
		func(d *Description) { d.Editions[0].Documents = d.Editions[0].Documents[:1] },
		func(d *Description) { d.Editions[0].Documents[0].Article = d.Editions[0].Documents[0].HTML },
		func(d *Description) { d.Editions[0].Documents[0].Images[0].Block = 1000000 },
	} {
		g := fixture(t)
		change(&g.d)
		mm := opened(t, encoded(t, g.d))
		if x, err := mm.OpenEdition(ctx, "one", g.index, indexLimits()); !errors.Is(err, ErrMismatch) || x != nil {
			t.Fatalf("index relation: %v", err)
		}
	}
	for _, change := range []func(*Description){
		func(d *Description) { d.Editions[0].Documents[0].Profile = "other/v1" },
		func(d *Description) { d.Editions[0].Documents[0].HTML = d.Editions[0].Documents[0].Article },
		func(d *Description) { d.Editions[0].Documents[0].Images = nil },
		func(d *Description) { d.Editions[0].Documents[0].Images[0].Block++ },
	} {
		g := fixture(t)
		change(&g.d)
		mm := opened(t, encoded(t, g.d))
		if _, err := mm.VerifyArticle(ctx, "one", 0, g.article, articleLimits()); !errors.Is(err, ErrMismatch) {
			t.Fatalf("article relation: %v", err)
		}
	}
	b := encoded(t, f.d)
	for _, pins := range [][2]string{{"other", f.d.Generation}, {f.d.Collection, "other"}} {
		if mm, err := Open(ctx, b, reference(b), pins[0], pins[1], testLimits()); !errors.Is(err, ErrMismatch) || mm != nil {
			t.Fatal("ignored metadata pins")
		}
	}
	r := reference(b)
	r.SHA256[0] ^= 1
	if mm, err := Open(ctx, b, r, f.d.Collection, f.d.Generation, testLimits()); !errors.Is(err, ErrMismatch) || mm != nil {
		t.Fatal("ignored independent bytes")
	}
}

func TestEmptyAndConcurrent(t *testing.T) {
	ctx := testContext(t)
	d := Description{Collection: "fixture-collection", Generation: "generation-1"}
	m := opened(t, encoded(t, d))
	if refs, err := m.Objects(ctx); err != nil || len(refs) != 0 {
		t.Fatal(err)
	}
	if _, err := m.OpenEdition(ctx, "empty", nil, indexLimits()); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	for _, zero := range []*Manifest{nil, {}} {
		if _, err := zero.Objects(ctx); !errors.Is(err, ErrInvalid) {
			t.Fatal("zero manifest objects", err)
		}
		if _, err := zero.Describe(ctx); !errors.Is(err, ErrInvalid) {
			t.Fatal("zero manifest description", err)
		}
		if _, err := zero.Resolve(ctx, "one", 0); !errors.Is(err, ErrInvalid) {
			t.Fatal("zero manifest binding", err)
		}
	}
	f := fixture(t)
	m = opened(t, encoded(t, f.d))
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 10 {
				if _, err := m.Objects(ctx); err != nil {
					t.Error(err)
				}
				if _, err := m.Describe(ctx); err != nil {
					t.Error(err)
				}
				if _, err := m.OpenEdition(ctx, "one", f.index, indexLimits()); err != nil {
					t.Error(err)
				}
				if _, err := m.VerifyArticle(ctx, "one", 0, f.article, articleLimits()); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	wg.Wait()
	for _, ctx := range []context.Context{nil, context.Background()} {
		if _, err := m.Objects(ctx); !errors.Is(err, ErrInvalid) {
			t.Fatal(err)
		}
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if out, err := m.Objects(canceled); !errors.Is(err, context.Canceled) || out != nil {
		t.Fatal(err)
	}
	if _, err := m.Describe(canceled); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := Encode(canceled, f.d, testLimits()); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := Open(canceled, nil, Ref{}, "", "", testLimits()); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := m.OpenEdition(canceled, "one", f.index, indexLimits()); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := m.VerifyArticle(canceled, "one", 0, f.article, articleLimits()); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func FuzzOpen(f *testing.F) {
	data := fixture(f)
	seed := encoded(f, data.d)
	f.Add(seed)
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"format":"\ud800"}`))
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) == 0 || len(b) > 65536 {
			return
		}
		ctx := testContext(t)
		l := testLimits()
		l.MaxEncodedBytes = 65536
		m, err := Open(ctx, b, reference(b), data.d.Collection, data.d.Generation, l)
		if err != nil {
			if m != nil {
				t.Fatal("partial result")
			}
			return
		}
		d, err := m.Describe(ctx)
		if err != nil {
			t.Fatal(err)
		}
		wire, err := Encode(ctx, d, l)
		if err != nil {
			t.Fatal(err)
		}
		n, err := Open(ctx, wire, reference(wire), data.d.Collection, data.d.Generation, l)
		if err != nil {
			t.Fatal(err)
		}
		dd, err := n.Describe(ctx)
		if err != nil || !reflect.DeepEqual(d, dd) {
			t.Fatal("unstable state")
		}
		again, err := Encode(ctx, dd, l)
		if err != nil || !bytes.Equal(wire, again) {
			t.Fatal("unstable writer")
		}
	})
}
