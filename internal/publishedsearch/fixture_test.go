package publishedsearch

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sidsharma96/concordance/internal/catalog"
	gm "github.com/sidsharma96/concordance/internal/generationmanifest"
	"github.com/sidsharma96/concordance/internal/generationpublish"
	"github.com/sidsharma96/concordance/internal/htmlarticle"
	"github.com/sidsharma96/concordance/internal/htmlarticlejson"
	"github.com/sidsharma96/concordance/internal/htmledition"
	"github.com/sidsharma96/concordance/internal/htmlpassage"
	"github.com/sidsharma96/concordance/internal/htmlterms"
	"github.com/sidsharma96/concordance/internal/objectstore"
	"github.com/sidsharma96/concordance/internal/termsearch"
	"github.com/sidsharma96/concordance/internal/textterms"
)

func testContext(t testing.TB) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)
	return ctx
}
func testLimits() Limits {
	return Limits{
		Manifest: gm.Limits{MaxEncodedBytes: 1 << 20, MaxSources: 4, MaxEditions: 4, MaxCaptures: 8, MaxFiles: 32, MaxDocuments: 32, MaxImages: 32, MaxSourceFiles: 32, MaxSourceDocuments: 32, MaxStringBytes: 4096, MaxValueBytes: 1 << 18, MaxObjectBytes: 1 << 20, MaxReferencedBytes: 8 << 20},
		Edition:  htmledition.Limits{MaxEncodedBytes: 1 << 20, MaxArticles: 32, MaxArticleBytes: 1 << 20, MaxBlocks: 10000, MaxPassages: 1000, MaxTerms: 20000, MaxPreparedBytes: 1 << 20, MaxKeyBytes: 1 << 20, MaxTermBytes: 1024, MaxVocabulary: 10000, MaxPostings: 20000},
		Article:  htmlarticlejson.Limits{MaxEncodedBytes: 1 << 20, MaxBlocks: 1000, MaxGroups: 1000, MaxStringBytes: 65536, MaxValueBytes: 1 << 20, MaxGroupDepth: 64, MaxHTMLBytes: 1 << 20},
	}
}
func queryLimits() termsearch.Limits {
	return termsearch.Limits{MaxEditions: 1, MaxPassages: 1000, MaxTerms: 20000, MaxQueryTerms: 16, MaxInputBytes: 1 << 20, MaxMatches: 1000}
}
func prepared(t testing.TB, word string) termsearch.Query {
	t.Helper()
	q, err := textterms.Prepare(testContext(t), word, textterms.Limits{MaxInputBytes: 1024, MaxTerms: 16, MaxTermBytes: 256, MaxOutputBytes: 4096})
	if err != nil {
		t.Fatal(err)
	}
	return termsearch.Query{TermsVersion: q.Version, Edition: "one", Terms: q.Terms}
}
func ref(b []byte) gm.Ref               { return gm.Ref{SHA256: sha256.Sum256(b), Bytes: int64(len(b))} }
func storeRef(r gm.Ref) objectstore.Ref { return objectstore.Ref{SHA256: r.SHA256, Bytes: r.Bytes} }

type bundle struct {
	description gm.Description
	manifest    []byte
	artifacts   [][]byte
	article     []byte
	index       *htmledition.Edition
}

// Synthetic trusted producer: two logical paths share article bytes; the empty
// edition has its own actual index. This does not certify a real renderer/capture.
func makeBundle(t testing.TB, generation, text string) bundle {
	t.Helper()
	ctx, l := testContext(t), testLimits()
	b := bundle{}
	seen := map[gm.Ref]bool{}
	put := func(body []byte) gm.Ref {
		r := ref(body)
		if !seen[r] {
			seen[r] = true
			b.artifacts = append(b.artifacts, bytes.Clone(body))
		}
		return r
	}
	html := []byte(`<main class="fixture"><h1>Pods</h1><p>` + text + `</p><img src="/images/pod.svg" alt="Pod diagram"></main>`)
	a, err := htmlarticle.Extract(ctx, html, htmlarticle.Profile{ID: "fixture/v1", ArticleClass: "fixture"}, htmlarticle.Limits{MaxInputBytes: 1 << 20, MaxTokenBytes: 65536, MaxTokens: 10000, MaxNodes: 10000, MaxDepth: 64, MaxBlocks: 1000, MaxGroups: 1000, MaxBlockBytes: 65536, MaxOutputBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	b.article, err = htmlarticlejson.Encode(ctx, a, l.Article)
	if err != nil {
		t.Fatal(err)
	}
	prep := htmlterms.Limits{Passages: htmlpassage.Limits{MaxBlocks: 1000, MaxGroups: 1000, MaxInputBytes: 1 << 20, MaxPassages: 1000, MaxBodyBytes: 4096, MaxReferences: 10000, MaxContextBytes: 1 << 20}, Field: textterms.Limits{MaxInputBytes: 65536, MaxTerms: 10000, MaxTermBytes: 1024, MaxOutputBytes: 1 << 20}, MaxViewBytes: 1 << 20, MaxTerms: 20000, MaxOutputBytes: 1 << 20}
	build := htmledition.BuildLimits{File: l.Edition, Article: l.Article, Preparation: prep}
	b.index, err = htmledition.Build(ctx, "one", [][]byte{b.article, b.article}, build)
	if err != nil {
		t.Fatal(err)
	}
	wire, err := htmledition.Encode(ctx, b.index, l.Edition)
	if err != nil {
		t.Fatal(err)
	}
	empty, err := htmledition.Build(ctx, "empty", nil, build)
	if err != nil {
		t.Fatal(err)
	}
	emptyWire, err := htmledition.Encode(ctx, empty, l.Edition)
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte("# Pods\n\n" + text + "\n")
	blob := fmt.Sprintf("%x", sha1.Sum(append([]byte(fmt.Sprintf("blob %d\x00", len(raw))), raw...)))
	c := gm.Capture{Source: "docs", Revision: strings.Repeat("a", 40), Directory: "docs", Preparation: "fixture/v1", Toolchain: "fixture/v1", Recipe: put([]byte("controlled synthetic producer")), Files: []gm.File{{Path: "docs/pods.md", BlobID: blob, Object: put(raw)}, {Path: "docs/other.md", BlobID: blob, Object: put(raw)}, {Path: "static/images/pod.svg", BlobID: strings.Repeat("b", 40), Object: put([]byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`))}}}
	var images []gm.Image
	for i, block := range a.Blocks {
		if block.Kind == htmlarticle.Image {
			images = append(images, gm.Image{Block: i, Address: block.SourceURL, File: 2})
		}
	}
	doc := gm.Document{Capture: 0, File: 0, NativeID: c.Files[0].Path, Location: c.Files[0].Path, Profile: a.Profile, HTML: put(html), Article: put(b.article), Images: images}
	other := doc
	other.File, other.NativeID, other.Location = 1, c.Files[1].Path, c.Files[1].Path
	other.Images = append([]gm.Image{}, images...)
	b.description = gm.Description{Collection: "guide", Generation: generation, Sources: []gm.Source{{ID: "docs", Repository: "https://example.invalid/docs.git"}}, Editions: []gm.Edition{{ID: "one", Index: put(wire), Captures: []gm.Capture{c}, Documents: []gm.Document{doc, other}}, {ID: "empty", Index: put(emptyWire)}}}
	b.encode(t)
	return b
}
func (b *bundle) encode(t testing.TB) {
	t.Helper()
	var err error
	b.manifest, err = gm.Encode(testContext(t), b.description, testLimits().Manifest)
	if err != nil {
		t.Fatal(err)
	}
}

type remote struct {
	mu     sync.Mutex
	bodies map[string][]byte
	calls  map[string]int
}

func (s *remote) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls[r.Method]++
	b, found := s.bodies[r.URL.Path]
	switch r.Method {
	case "GET", "HEAD":
		if !found {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(404)
			_, _ = io.WriteString(w, "<Error><Code>NoSuchKey</Code></Error>")
			return
		}
		tag := fmt.Sprintf(`"%x"`, sha256.Sum256(b))
		if v := r.Header.Get("If-Match"); v != "" && v != tag {
			w.WriteHeader(412)
			return
		}
		w.Header().Set("ETag", tag)
		w.Header().Set("Content-Length", fmt.Sprint(len(b)))
		if r.Method == "GET" {
			_, _ = w.Write(b)
		}
	case "PUT":
		if r.Header.Get("If-None-Match") != "*" {
			w.WriteHeader(400)
			return
		}
		if found {
			w.WriteHeader(412)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, (1<<20)+1))
		if err != nil || len(body) > 1<<20 {
			w.WriteHeader(400)
			return
		}
		s.bodies[r.URL.Path] = body
	default:
		w.WriteHeader(405)
	}
}
func (s *remote) reset() { s.mu.Lock(); defer s.mu.Unlock(); s.calls = map[string]int{} }
func (s *remote) counts() map[string]int {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]int{}
	for k, v := range s.calls {
		out[k] = v
	}
	return out
}

type fixture struct {
	cat       *catalog.Store
	pool      *pgxpool.Pool
	objects   *objectstore.Store
	namespace catalog.Namespace
	publisher *generationpublish.Publisher
	remote    *remote
	server    *httptest.Server
}

func setup(t *testing.T) fixture {
	t.Helper()
	dsn := os.Getenv("CONCORDANCE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set CONCORDANCE_TEST_DATABASE_URL to a dedicated local test database")
	}
	ctx := testContext(t)
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	schema := pgx.Identifier{"reader_" + strings.ToLower(rand.Text())}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Error(err)
		}
	})
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	cfg.MaxConns = 4
	f := fixture{remote: &remote{bodies: map[string][]byte{}, calls: map[string]int{}}}
	f.pool, err = pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(f.pool.Close)
	f.cat, err = catalog.New(f.pool)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.cat.CreateSchema(ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.cat.CreateReceiptSchema(ctx); err != nil {
		t.Fatal(err)
	}
	f.server = httptest.NewServer(f.remote)
	t.Cleanup(f.server.Close)
	f.objects = f.newObjects(t, "guide")
	f.namespace, err = f.cat.BindNamespace(ctx, "fixture", f.objects.Binding())
	if err != nil {
		t.Fatal(err)
	}
	l := testLimits()
	f.publisher, err = generationpublish.New(f.cat, f.objects, f.namespace, generationpublish.Limits{MaxObjects: 64, MaxBatchObjects: 8, MaxInputBytes: 8 << 20, MaxBatchBytes: 1 << 20, Manifest: l.Manifest, Edition: l.Edition, Article: l.Article})
	if err != nil {
		t.Fatal(err)
	}
	return f
}
func (f fixture) newObjects(t testing.TB, collection string) *objectstore.Store {
	t.Helper()
	client := s3.New(s3.Options{Region: "auto", BaseEndpoint: aws.String(f.server.URL), UsePathStyle: true, Credentials: aws.AnonymousCredentials{}, HTTPClient: f.server.Client()})
	s, err := objectstore.New(client, "fixture", collection, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func (f fixture) publish(t testing.TB, b bundle) {
	t.Helper()
	r := catalog.Request{Collection: "guide", Key: b.description.Generation, FingerprintVersion: "synthetic/v1", Fingerprint: sha256.Sum256([]byte(b.description.Generation))}
	if _, err := f.publisher.Publish(testContext(t), r, b.description.Generation, b.manifest, b.artifacts); err != nil {
		t.Fatal(err)
	}
}
func (f fixture) open(t testing.TB, generation string) *Reader {
	t.Helper()
	r, err := Open(testContext(t), f.cat, f.newObjects(t, "guide"), Selection{"guide", generation, "one"}, testLimits())
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func (f fixture) path(t testing.TB, r gm.Ref) string {
	t.Helper()
	key, err := f.objects.Key(storeRef(r))
	if err != nil {
		t.Fatal(err)
	}
	return "/fixture/" + key
}
