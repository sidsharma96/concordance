package preparedimport

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
	"github.com/sidsharma96/concordance/internal/generationpublish"
	"github.com/sidsharma96/concordance/internal/objectstore"
	"github.com/sidsharma96/concordance/internal/termsearch"
	"github.com/sidsharma96/concordance/internal/textterms"
)

type remote struct {
	mu     sync.Mutex
	bodies map[string][]byte
	calls  [3]int // GET, HEAD, PUT
	before func(*http.Request) int
}

func (r *remote) counts() [3]int { r.mu.Lock(); defer r.mu.Unlock(); return r.calls }
func (r *remote) ServeHTTP(w http.ResponseWriter, q *http.Request) {
	r.mu.Lock()
	switch q.Method {
	case "GET":
		r.calls[0]++
	case "HEAD":
		r.calls[1]++
	case "PUT":
		r.calls[2]++
	}
	hook := r.before
	r.mu.Unlock()
	if hook != nil {
		if status := hook(q); status != 0 {
			w.WriteHeader(status)
			return
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	body, exists := r.bodies[q.URL.Path]
	switch q.Method {
	case "PUT":
		if q.Header.Get("If-None-Match") != "*" {
			w.WriteHeader(400)
			return
		}
		if exists {
			w.WriteHeader(412)
			return
		}
		b, err := io.ReadAll(io.LimitReader(q.Body, (1<<20)+1))
		if err != nil || len(b) > 1<<20 {
			w.WriteHeader(400)
			return
		}
		r.bodies[q.URL.Path] = b
	case "GET", "HEAD":
		if !exists {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(404)
			if q.Method == "GET" {
				_, _ = io.WriteString(w, "<Error><Code>NoSuchKey</Code></Error>")
			}
			return
		}
		tag := fmt.Sprintf(`"opaque-%x"`, sha256.Sum256(body))
		if v := q.Header.Get("If-Match"); v != "" && v != tag {
			w.WriteHeader(412)
			return
		}
		w.Header().Set("ETag", tag)
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		if q.Method == "GET" {
			_, _ = w.Write(body)
		}
	default:
		w.WriteHeader(405)
	}
}

type storageFixture struct {
	fixture
	cat      *catalog.Store
	pool     *pgxpool.Pool
	config   *pgxpool.Config
	ns       catalog.Namespace
	remote   *remote
	server   *httptest.Server
	importer *Importer
}

func setup(t *testing.T) storageFixture {
	t.Helper()
	dsn := os.Getenv("CONCORDANCE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set CONCORDANCE_TEST_DATABASE_URL to a dedicated test database")
	}
	f := storageFixture{fixture: newFixture(t), remote: &remote{bodies: map[string][]byte{}}}
	admin, err := pgxpool.New(f.ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	schema := pgx.Identifier{"prepared_" + strings.ToLower(rand.Text())}.Sanitize()
	if _, err := admin.Exec(f.ctx, "CREATE SCHEMA "+schema); err != nil {
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
	f.config = cfg
	f.cat, f.pool = f.catalog(t)
	if err := f.cat.CreateSchema(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.cat.CreateReceiptSchema(f.ctx); err != nil {
		t.Fatal(err)
	}
	f.server = httptest.NewServer(f.remote)
	t.Cleanup(f.server.Close)
	o := f.objects(t)
	f.ns, err = f.cat.BindNamespace(f.ctx, "fixture", o.Binding())
	if err != nil {
		t.Fatal(err)
	}
	p, err := generationpublish.New(f.cat, o, f.ns, publicationLimits())
	if err != nil {
		t.Fatal(err)
	}
	f.importer, err = New(p, "guide", f.sources, testLimits())
	if err != nil {
		t.Fatal(err)
	}
	return f
}
func (f storageFixture) catalog(t *testing.T) (*catalog.Store, *pgxpool.Pool) {
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
func (f storageFixture) objects(t *testing.T) *objectstore.Store {
	t.Helper()
	sdk := s3.New(s3.Options{Region: "auto", BaseEndpoint: aws.String(f.server.URL), UsePathStyle: true, Credentials: aws.AnonymousCredentials{}, HTTPClient: f.server.Client()})
	o, err := objectstore.New(sdk, "fixture", "guide", 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	return o
}
func stored(r gm.Ref) objectstore.Ref { return objectstore.Ref{SHA256: r.SHA256, Bytes: r.Bytes} }

func TestPreparedImportPublishesRecoversAndReopens(t *testing.T) {
	f := setup(t)
	f.remote.before = func(q *http.Request) int {
		record, err := f.cat.Lookup(f.ctx, "guide", "saved")
		if err != nil || record.Generation != nil {
			t.Error("object I/O before binding or after publication", err)
			return 500
		}
		return 0
	}
	first, err := f.importer.ImportPrepared(f.ctx, "saved", "candidate", f.editions)
	if err != nil || first.Duplicate {
		t.Fatal(first, err)
	}
	// All request hooks finished before the synchronous call returned.
	f.remote.mu.Lock()
	f.remote.before = nil
	f.remote.mu.Unlock()
	count := f.remote.counts()
	n := count[2]
	if count != [3]int{2 * n, 0, n} || n == 0 {
		t.Fatal("cold confirmation sequence", count)
	}
	retry, err := f.importer.ImportPrepared(f.ctx, "saved", "different-candidate", f.editions)
	if err != nil || !retry.Duplicate || retry.Generation != first.Generation || f.remote.counts() != count {
		t.Fatal("completed retry did object I/O", retry, err)
	}
	changed := f.clone()
	changed[0].Captures[0].Recipe = append(changed[0].Captures[0].Recipe, []byte("; changed build recipe")...)
	if out, err := f.importer.ImportPrepared(f.ctx, "saved", "changed", changed); !errors.Is(err, catalog.ErrConflict) || out != (catalog.Publication{}) || f.remote.counts() != count {
		t.Fatal(out, err)
	}
	// Clear all caller buffers and remove the local source before fresh readers.
	for _, e := range f.editions {
		for _, c := range e.Captures {
			clear(c.Recipe)
			for _, p := range c.Pages {
				clear(p.HTML)
			}
		}
	}
	if err := os.RemoveAll(f.repo); err != nil {
		t.Fatal(err)
	}
	f.pool.Close()
	c, _ := f.catalog(t)
	published, err := c.LookupGeneration(f.ctx, "guide", "candidate")
	if err != nil || published != first.Generation {
		t.Fatal(published, err)
	}
	o := f.objects(t)
	root := gm.Ref{SHA256: published.ManifestSHA256, Bytes: published.ManifestBytes}
	body, err := o.Read(f.ctx, stored(root))
	if err != nil {
		t.Fatal(err)
	}
	m, err := gm.Open(f.ctx, body, root, "guide", published.ID, testLimits().Manifest)
	if err != nil {
		t.Fatal(err)
	}
	d, err := m.Describe(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	indexBody, err := o.Read(f.ctx, stored(d.Editions[0].Index))
	if err != nil {
		t.Fatal(err)
	}
	index, err := m.OpenEdition(f.ctx, "one", indexBody, testLimits().Edition.File)
	if err != nil {
		t.Fatal(err)
	}
	q, err := textterms.Prepare(f.ctx, "retry", testLimits().Edition.Preparation.Field)
	if err != nil {
		t.Fatal(err)
	}
	hits, err := index.Search(f.ctx, termsearch.Query{TermsVersion: q.Version, Edition: "one", Terms: q.Terms}, searchLimits())
	if err != nil || len(hits.Matches) != 2 {
		t.Fatal(hits, err)
	}
	loc, err := index.Location(hits.Matches[0].Passage)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := m.Resolve(f.ctx, "one", loc.Article)
	if err != nil || binding.NativeID != "docs/a.md" || binding.Revision != f.commit {
		t.Fatal(binding, err)
	}
	article, err := o.Read(f.ctx, stored(binding.Article))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.VerifyArticle(f.ctx, "one", loc.Article, article, testLimits().Edition.Article); err != nil {
		t.Fatal(err)
	}
	quote, err := index.Quote(f.ctx, hits.Matches[0].Passage, article, testLimits().Edition.Article, 4096)
	if err != nil || !strings.Contains(quote, "Retry uploads safely.") {
		t.Fatal(quote, err)
	}
	t.Logf("published: documents=2; cold GET=%d HEAD=%d PUT=%d; completed retry adds no object requests", count[0], count[1], count[2])
	t.Logf("fresh-reader citation: %s at captured commit; quote=%q", binding.NativeID, quote)
}

func TestPreparedImportFailureNeverBindsOrWrites(t *testing.T) {
	f := setup(t)
	for _, name := range []string{"missing page", "missing dependency", "broken HTML", "small hash budget"} {
		t.Run(name, func(t *testing.T) {
			editions := f.clone()
			c := &editions[0].Captures[0]
			l := testLimits()
			switch name {
			case "missing page":
				c.Pages = c.Pages[:1]
			case "missing dependency":
				c.Dependencies = append(c.Dependencies, "missing.yaml")
			case "broken HTML":
				c.Pages[0].HTML = []byte("<p>No selected article</p>")
			case "small hash budget":
				l.MaxHashBytes = 1
			}
			p, err := generationpublish.New(f.cat, f.objects(t), f.ns, publicationLimits())
			if err != nil {
				t.Fatal(err)
			}
			i, err := New(p, "guide", f.sources, l)
			if err != nil {
				t.Fatal(err)
			}
			out, err := i.ImportPrepared(f.ctx, name, "candidate", editions)
			if err == nil || out != (catalog.Publication{}) || f.remote.counts() != [3]int{} {
				t.Fatal("invalid preparation had remote effects", out, err)
			}
			if _, err := f.cat.Lookup(f.ctx, "guide", name); !errors.Is(err, catalog.ErrNotFound) {
				t.Fatal("invalid preparation bound a key", err)
			}
		})
	}
}

func TestPreparedImportRemoteFailureKeepsUnpublishedBinding(t *testing.T) {
	f := setup(t)
	f.remote.before = func(q *http.Request) int {
		if q.Method == "PUT" {
			return 403
		}
		return 0
	}
	out, err := f.importer.ImportPrepared(f.ctx, "saved", "candidate", f.editions)
	if err == nil || out != (catalog.Publication{}) {
		t.Fatal(out, err)
	}
	record, err := f.cat.Lookup(f.ctx, "guide", "saved")
	if err != nil || record.Generation != nil {
		t.Fatal("failed upload published", record, err)
	}
	f.remote.mu.Lock()
	f.remote.before = nil
	f.remote.mu.Unlock()
	out, err = f.importer.ImportPrepared(f.ctx, "saved", "retried", f.editions)
	if err != nil || out.Duplicate || out.Generation.ID != "retried" {
		t.Fatal("explicit retry failed", out, err)
	}
}

func TestPreparedImportDoesNotQueueConcurrentCaller(t *testing.T) {
	f := setup(t)
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	f.remote.before = func(q *http.Request) int { once.Do(func() { close(entered); <-release }); return 0 }
	type result struct {
		out catalog.Publication
		err error
	}
	done := make(chan result, 1)
	go func() {
		out, err := f.importer.ImportPrepared(f.ctx, "first", "candidate", f.editions)
		done <- result{out, err}
	}()
	select {
	case <-entered:
	case r := <-done:
		t.Fatal("first call finished before controlled I/O boundary", r)
	case <-f.ctx.Done():
		close(release)
		<-done
		t.Fatal(f.ctx.Err())
	}
	out, err := f.importer.ImportPrepared(f.ctx, "second", "other", f.editions)
	close(release)
	first := <-done
	if !errors.Is(err, ErrBusy) || out != (catalog.Publication{}) || first.err != nil {
		t.Fatal(out, err, first)
	}
	if _, err := f.cat.Lookup(f.ctx, "guide", "second"); !errors.Is(err, catalog.ErrNotFound) {
		t.Fatal("busy caller bound key", err)
	}
}
