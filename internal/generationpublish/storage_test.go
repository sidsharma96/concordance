package generationpublish

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
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
	"github.com/sidsharma96/concordance/internal/objectstore"
	"github.com/sidsharma96/concordance/internal/termsearch"
	"github.com/sidsharma96/concordance/internal/textterms"
)

type remote struct {
	mu     sync.Mutex
	bodies map[string][]byte
	calls  [3]int // GET, HEAD, PUT
	hook   func(*http.Request) int
}

func (s *remote) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	switch r.Method {
	case "GET":
		s.calls[0]++
	case "HEAD":
		s.calls[1]++
	case "PUT":
		s.calls[2]++
	}
	hook := s.hook
	s.mu.Unlock()
	if hook != nil {
		if status := hook(r); status != 0 {
			w.WriteHeader(status)
			return
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	body, exists := s.bodies[r.URL.Path]
	switch r.Method {
	case "PUT":
		if r.Header.Get("If-None-Match") != "*" {
			w.WriteHeader(400)
			return
		}
		if exists {
			w.WriteHeader(412)
			return
		}
		b, err := io.ReadAll(io.LimitReader(r.Body, (1<<20)+1))
		if err != nil || len(b) > 1<<20 {
			w.WriteHeader(400)
			return
		}
		s.bodies[r.URL.Path] = b
	case "GET", "HEAD":
		if !exists {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(404)
			if r.Method == "GET" {
				_, _ = io.WriteString(w, "<Error><Code>NoSuchKey</Code></Error>")
			}
			return
		}
		tag := fmt.Sprintf(`"opaque-%x"`, sha256.Sum256(body))
		if v := r.Header.Get("If-Match"); v != "" && v != tag {
			w.WriteHeader(412)
			return
		}
		w.Header().Set("ETag", tag)
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		if r.Method == "GET" {
			_, _ = w.Write(body)
		}
	default:
		w.WriteHeader(405)
	}
}
func (s *remote) setHook(hook func(*http.Request) int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hook = hook
}
func (s *remote) counts() [3]int { s.mu.Lock(); defer s.mu.Unlock(); return s.calls }
func (s *remote) size() int      { s.mu.Lock(); defer s.mu.Unlock(); return len(s.bodies) }

type storageFixture struct {
	ctx       context.Context
	cat       *catalog.Store
	pool      *pgxpool.Pool
	ns        catalog.Namespace
	objects   *objectstore.Store
	publisher *Publisher
	remote    *remote
	server    *httptest.Server
	config    *pgxpool.Config
}

func setup(t *testing.T) storageFixture {
	return setupPeer(t, nil)
}

func setupPeer(t *testing.T, wrap func(http.Handler) http.Handler) storageFixture {
	t.Helper()
	dsn := os.Getenv("CONCORDANCE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set CONCORDANCE_TEST_DATABASE_URL to a dedicated test database")
	}
	ctx := testContext(t)
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	name := "publish_" + strings.ToLower(rand.Text())
	schema := pgx.Identifier{name}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := admin.Exec(c, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Error(err)
		}
	})
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	cfg.ConnConfig.RuntimeParams["application_name"] = name
	cfg.MaxConns = 4
	f := storageFixture{ctx: ctx, config: cfg, remote: &remote{bodies: map[string][]byte{}}}
	var handler http.Handler = f.remote
	if wrap != nil {
		handler = wrap(handler)
	}
	f.server = httptest.NewServer(handler)
	t.Cleanup(f.server.Close)
	f.cat, f.pool = f.reopenCatalog(t)
	if err := f.cat.CreateSchema(ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.cat.CreateReceiptSchema(ctx); err != nil {
		t.Fatal(err)
	}
	f.objects = f.freshObjects(t)
	f.ns, err = f.cat.BindNamespace(ctx, "fixture", f.objects.Binding())
	if err != nil {
		t.Fatal(err)
	}
	f.publisher, err = New(f.cat, f.objects, f.ns, testLimits())
	if err != nil {
		t.Fatal(err)
	}
	return f
}
func (f storageFixture) reopenCatalog(t *testing.T) (*catalog.Store, *pgxpool.Pool) {
	t.Helper()
	pool, err := pgxpool.NewWithConfig(f.ctx, f.config.Copy())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	c, err := catalog.New(pool)
	if err != nil {
		t.Fatal(err)
	}
	return c, pool
}
func (f storageFixture) freshObjects(t *testing.T) *objectstore.Store {
	t.Helper()
	sdk := s3.New(s3.Options{Region: "auto", BaseEndpoint: aws.String(f.server.URL), UsePathStyle: true, Credentials: aws.AnonymousCredentials{}, HTTPClient: f.server.Client()})
	o, err := objectstore.New(sdk, "fixture", "guide", 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	return o
}
func (f storageFixture) unpublished(t *testing.T, key string) {
	t.Helper()
	r, err := f.cat.Lookup(f.ctx, "guide", key)
	if err != nil || r.Generation != nil {
		t.Fatal("must retain unpublished binding", r, err)
	}
}
func (f storageFixture) rootPath(t *testing.T, b bundle) string {
	t.Helper()
	key, err := f.objects.Key(storeRef(ref(b.manifest)))
	if err != nil {
		t.Fatal(err)
	}
	return "/fixture/" + key
}
func runPublish(f storageFixture, b bundle, key string) (catalog.Publication, error) {
	return f.publisher.Publish(f.ctx, trustedRequest(key), b.d.Generation, b.manifest, b.artifacts)
}

func TestPublishedGenerationReopensWithoutWriterBuffers(t *testing.T) {
	f := setup(t)
	b := makeBundle(t)
	// Observing every S3 request also checks that binding committed first and that
	// publication is still absent. No database transaction spans an HTTP operation.
	f.remote.setHook(func(r *http.Request) int {
		record, err := f.cat.Lookup(f.ctx, "guide", "first")
		if err != nil || record.Generation != nil {
			t.Error("upload before binding or after publication", err)
			return 500
		}
		return 0
	})
	first, err := runPublish(f, b, "first")
	if err != nil || first.Duplicate {
		t.Fatal(first, err)
	}
	f.remote.setHook(nil)
	n := len(b.artifacts) + 1
	if got := f.remote.counts(); got != [3]int{2 * n, 0, n} {
		t.Fatal("cold confirmations", got)
	}
	if f.remote.size() != n {
		t.Fatal("did not store complete generation")
	}
	clear(b.manifest)
	for _, body := range b.artifacts {
		clear(body)
	}
	f.pool.Close()
	c, _ := f.reopenCatalog(t)
	o := f.freshObjects(t)
	published, err := c.LookupGeneration(f.ctx, "guide", first.Generation.ID)
	if err != nil || published != first.Generation {
		t.Fatal(published, err)
	}
	root := objectstore.Ref{SHA256: published.ManifestSHA256, Bytes: published.ManifestBytes}
	key, err := o.Key(root)
	if err != nil || key != published.ManifestKey {
		t.Fatal("catalog physical binding", err)
	}
	before := f.remote.counts()
	body, err := o.Read(f.ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	m, err := gm.Open(f.ctx, body, manifestRef(root), "guide", published.ID, testLimits().Manifest)
	if err != nil {
		t.Fatal(err)
	}
	clear(body)
	d, err := m.Describe(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	indexBody, err := f.freshObjects(t).Read(f.ctx, storeRef(d.Editions[0].Index))
	if err != nil {
		t.Fatal(err)
	}
	index, err := m.OpenEdition(f.ctx, "one", indexBody, testLimits().Edition)
	if err != nil {
		t.Fatal(err)
	}
	clear(indexBody)
	q, err := textterms.Prepare(f.ctx, "retry", textterms.Limits{MaxInputBytes: 256, MaxTerms: 16, MaxTermBytes: 256, MaxOutputBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	hits, err := index.Search(f.ctx, termsearch.Query{TermsVersion: q.Version, Edition: "one", Terms: q.Terms}, termsearch.Limits{MaxEditions: 1, MaxPassages: 1000, MaxTerms: 20000, MaxQueryTerms: 16, MaxInputBytes: 1 << 20, MaxMatches: 1000})
	if err != nil || len(hits.Matches) != 2 {
		t.Fatal(hits, err)
	}
	loc, err := index.Location(hits.Matches[0].Passage)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := m.Resolve(f.ctx, "one", loc.Article)
	if err != nil || binding.NativeID != "docs/pods.md" {
		t.Fatal(binding, err)
	}
	article, err := f.freshObjects(t).Read(f.ctx, storeRef(binding.Article))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.VerifyArticle(f.ctx, "one", loc.Article, article, testLimits().Article); err != nil {
		t.Fatal(err)
	}
	quote, err := index.Quote(f.ctx, hits.Matches[0].Passage, article, testLimits().Article, 4096)
	if err != nil || !strings.Contains(quote, "Retry uploads safely.") {
		t.Fatal(quote, err)
	}
	clear(article)
	after := f.remote.counts()
	if after != [3]int{before[0] + 3, before[1], before[2]} {
		t.Fatal("reader must fetch manifest/index/article only", before, after)
	}
	t.Logf("published %d artifacts including root; cold GET=%d PUT=%d; fresh quote GET=3", n, before[0], before[2])
}
func TestRetriesAndWarmObjects(t *testing.T) {
	f := setup(t)
	b := makeBundle(t)
	first, err := runPublish(f, b, "first")
	if err != nil {
		t.Fatal(err)
	}
	before := f.remote.counts()
	b.d.Generation = "unused-candidate"
	b.encode(t)
	duplicate, err := runPublish(f, b, "first")
	if err != nil || !duplicate.Duplicate || duplicate.Generation != first.Generation {
		t.Fatal(duplicate, err)
	}
	if got := f.remote.counts(); got != before {
		t.Fatal("completed retry accessed objects", got, before)
	}
	changed := trustedRequest("first")
	changed.Fingerprint[0] ^= 1
	if out, err := f.publisher.Publish(f.ctx, changed, b.d.Generation, b.manifest, b.artifacts); !errors.Is(err, catalog.ErrConflict) || out != (catalog.Publication{}) {
		t.Fatal(out, err)
	}
	if got := f.remote.counts(); got != before {
		t.Fatal("conflict accessed objects")
	}
	second, err := runPublish(f, b, "second")
	if err != nil || second.Duplicate || second.Generation.ID != b.d.Generation {
		t.Fatal(second, err)
	}
	want := [3]int{before[0] + 2, before[1] + len(b.artifacts), before[2] + 1}
	if got := f.remote.counts(); got != want {
		t.Fatal("warm artifacts should HEAD; new root strict", got, want)
	}
	if err := f.cat.Quarantine(f.ctx, f.ns); err != nil {
		t.Fatal(err)
	}
	before = f.remote.counts()
	out, err := runPublish(f, b, "first")
	if err != nil || !out.Duplicate || out.Generation != first.Generation {
		t.Fatal("quarantine lost completed result", out, err)
	}
	if got := f.remote.counts(); got != before {
		t.Fatal("quarantined completed retry did remote work")
	}
}
func TestUploadAndCatalogFailuresDoNotPublish(t *testing.T) {
	for _, rootOnly := range []bool{false, true} {
		t.Run(fmt.Sprintf("root-only-%t", rootOnly), func(t *testing.T) {
			f := setup(t)
			b := makeBundle(t)
			path := f.rootPath(t, b)
			f.remote.setHook(func(r *http.Request) int {
				if r.Method == "PUT" && (!rootOnly || r.URL.Path == path) {
					return 503
				}
				return 0
			})
			out, err := runPublish(f, b, "failed")
			if err == nil || out != (catalog.Publication{}) {
				t.Fatal(out, err)
			}
			f.unpublished(t, "failed")
			if rootOnly && f.remote.size() != len(b.artifacts) {
				t.Fatal("confirmed objects should remain")
			}
			f.remote.setHook(nil)
			recovered, err := runPublish(f, b, "failed")
			if err != nil || recovered.Duplicate {
				t.Fatal("explicit same-input retry", recovered, err)
			}
		})
	}
	t.Run("catalog unavailable before binding", func(t *testing.T) {
		f := setup(t)
		b := makeBundle(t)
		f.pool.Close()
		if out, err := runPublish(f, b, "failed"); err == nil || out != (catalog.Publication{}) {
			t.Fatal(out, err)
		}
		if f.remote.counts() != [3]int{} {
			t.Fatal("unbound upload")
		}
	})
	t.Run("publication rejected", func(t *testing.T) {
		f := setup(t)
		b := makeBundle(t)
		_, err := f.pool.Exec(f.ctx, `CREATE FUNCTION reject_publication() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'fixture publication rejection'; END $$;
   CREATE TRIGGER reject_publication BEFORE UPDATE ON concordance_imports FOR EACH ROW EXECUTE FUNCTION reject_publication()`)
		if err != nil {
			t.Fatal(err)
		}
		if out, err := runPublish(f, b, "failed"); err == nil || out != (catalog.Publication{}) {
			t.Fatal(out, err)
		}
		f.unpublished(t, "failed")
		if f.remote.size() != len(b.artifacts)+1 {
			t.Fatal("remote objects lost after SQL rejection")
		}
		if _, err := f.pool.Exec(f.ctx, "DROP TRIGGER reject_publication ON concordance_imports"); err != nil {
			t.Fatal(err)
		}
		if out, err := runPublish(f, b, "failed"); err != nil || out.Duplicate {
			t.Fatal(out, err)
		}
	})
	t.Run("missing known object quarantines", func(t *testing.T) {
		f := setup(t)
		b := makeBundle(t)
		if _, err := runPublish(f, b, "first"); err != nil {
			t.Fatal(err)
		}
		r := ref(b.artifacts[0])
		key, err := f.objects.Key(storeRef(r))
		if err != nil {
			t.Fatal(err)
		}
		f.remote.mu.Lock()
		delete(f.remote.bodies, "/fixture/"+key)
		f.remote.mu.Unlock()
		b.d.Generation = "second"
		b.encode(t)
		if out, err := runPublish(f, b, "second"); !errors.Is(err, objectstore.ErrChanged) || out != (catalog.Publication{}) {
			t.Fatal(out, err)
		}
		f.unpublished(t, "second")
		if err := f.cat.CheckNamespace(f.ctx, f.ns); !errors.Is(err, catalog.ErrNamespace) {
			t.Fatal(err)
		}
	})
}

func TestStoredPutErrorIsReconciledBeforePublication(t *testing.T) {
	f := setup(t)
	b := makeBundle(t)
	f.remote.setHook(func(r *http.Request) int {
		if r.Method != "PUT" {
			return 0
		}
		// The service stores the bytes but returns an error. The client must verify
		// the same immutable bytes, not assume either success or rollback.
		body, err := io.ReadAll(io.LimitReader(r.Body, (1<<20)+1))
		if err != nil || len(body) > 1<<20 {
			t.Error("fixture read", err)
			return 400
		}
		f.remote.mu.Lock()
		f.remote.bodies[r.URL.Path] = body
		f.remote.mu.Unlock()
		return 503
	})
	out, err := runPublish(f, b, "ambiguous-put")
	if err != nil || out.Duplicate {
		t.Fatal(out, err)
	}
	n := len(b.artifacts) + 1
	if f.remote.counts() != [3]int{2 * n, 0, n} {
		t.Fatal("must reconcile every stored/error PUT exactly once", f.remote.counts())
	}
}
func TestInvalidBundleDoesNotBindOrUpload(t *testing.T) {
	f := setup(t)
	b := makeBundle(t)
	b.artifacts = b.artifacts[1:]
	if out, err := runPublish(f, b, "invalid"); !errors.Is(err, ErrInvalid) || out != (catalog.Publication{}) {
		t.Fatal(out, err)
	}
	if f.remote.counts() != [3]int{} {
		t.Fatal("invalid input touched object store")
	}
	if _, err := f.cat.Lookup(f.ctx, "guide", "invalid"); !errors.Is(err, catalog.ErrNotFound) {
		t.Fatal("invalid input bound a key", err)
	}
}
