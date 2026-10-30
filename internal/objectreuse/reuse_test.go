package objectreuse

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
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
	"github.com/sidsharma96/concordance/internal/objectstore"
)

type remote struct {
	mu                sync.Mutex
	objects           map[string][]byte
	gets, heads, puts int
	headHook          func()
}

func (r *remote) ServeHTTP(w http.ResponseWriter, q *http.Request) {
	if q.Method == "HEAD" && r.headHook != nil {
		r.headHook()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	b, ok := r.objects[q.URL.Path]
	switch q.Method {
	case "GET":
		r.gets++
	case "HEAD":
		r.heads++
	case "PUT":
		r.puts++
		if q.Header.Get("If-None-Match") != "*" {
			w.WriteHeader(400)
			return
		}
		if ok {
			w.WriteHeader(412)
			return
		}
		b, _ = io.ReadAll(q.Body)
		r.objects[q.URL.Path] = bytes.Clone(b)
		return
	default:
		w.WriteHeader(405)
		return
	}
	if !ok {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(404)
		if q.Method == "GET" {
			fmt.Fprint(w, "<Error><Code>NoSuchKey</Code></Error>")
		}
		return
	}
	tag := fmt.Sprintf(`"opaque-%x"`, sha256.Sum256(b))
	if expected := q.Header.Get("If-Match"); expected != "" && expected != tag {
		w.WriteHeader(412)
		return
	}
	w.Header().Set("ETag", tag)
	w.Header().Set("Content-Length", strconv.Itoa(len(b)))
	if q.Method == "GET" {
		_, _ = w.Write(b)
	}
}
func (r *remote) counts() (int, int, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.gets, r.heads, r.puts
}

type fixture struct {
	ctx     context.Context
	cat     *catalog.Store
	pool    *pgxpool.Pool
	ns      catalog.Namespace
	objects *objectstore.Store
	reuse   *Store
	remote  *remote
	config  *pgxpool.Config
}

func setup(t *testing.T) fixture {
	t.Helper()
	dsn := os.Getenv("CONCORDANCE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set dedicated test database")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)
	admin, e := pgxpool.New(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(admin.Close)
	schema := "reuse_" + strings.ToLower(rand.Text())
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, e = admin.Exec(ctx, "CREATE SCHEMA "+quoted); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		c, x := context.WithTimeout(context.Background(), 10*time.Second)
		defer x()
		if _, e := admin.Exec(c, "DROP SCHEMA "+quoted+" CASCADE"); e != nil {
			t.Error(e)
		}
	})
	cfg, e := pgxpool.ParseConfig(dsn)
	if e != nil {
		t.Fatal(e)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = quoted
	cfg.MaxConns = 4
	cfg.ConnConfig.RuntimeParams["application_name"] = schema
	pool, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(pool.Close)
	c, _ := catalog.New(pool)
	if e = c.CreateSchema(ctx); e != nil {
		t.Fatal(e)
	}
	if e = c.CreateReceiptSchema(ctx); e != nil {
		t.Fatal(e)
	}
	remote := &remote{objects: make(map[string][]byte)}
	server := httptest.NewServer(remote)
	t.Cleanup(server.Close)
	client := s3.New(s3.Options{Region: "auto", BaseEndpoint: aws.String(server.URL), UsePathStyle: true, Credentials: aws.AnonymousCredentials{}, HTTPClient: server.Client()})
	objects, e := objectstore.New(client, "bucket", "guide", 1024)
	if e != nil {
		t.Fatal(e)
	}
	n, e := c.BindNamespace(ctx, "fixture", objects.Binding())
	if e != nil {
		t.Fatal(e)
	}
	reuse, e := New(c, objects, n, Limits{MaxObjects: 4, MaxInputBytes: 2048})
	if e != nil {
		t.Fatal(e)
	}
	return fixture{ctx, c, pool, n, objects, reuse, remote, cfg}
}
func (f fixture) request(key string) catalog.Request {
	return catalog.Request{Collection: "guide", Key: key, FingerprintVersion: "fixture", Fingerprint: sha256.Sum256([]byte(key))}
}
func candidate() catalog.Generation {
	return catalog.Generation{ID: "generation", ManifestKey: "illustrative-not-a-complete-manifest", ManifestBytes: 1}
}

func TestStrictThenFreshPoolReuseAndRead(t *testing.T) {
	f := setup(t)
	bodies := [][]byte{[]byte("alpha"), []byte("beta"), []byte("alpha"), nil}
	refs, e := f.reuse.Ensure(f.ctx, bodies)
	if e != nil || len(refs) != 4 || refs[0] != refs[2] {
		t.Fatal(refs, e)
	}
	g, h, p := f.remote.counts()
	if g != 6 || h != 0 || p != 3 {
		t.Fatal(g, h, p)
	}
	f.pool.Close()
	pool, e := pgxpool.NewWithConfig(f.ctx, f.config.Copy())
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(pool.Close)
	c, _ := catalog.New(pool)
	ns, e := c.BindNamespace(f.ctx, "fixture", f.objects.Binding())
	if e != nil {
		t.Fatal(e)
	}
	reuse, _ := New(c, f.objects, ns, Limits{MaxObjects: 4, MaxInputBytes: 2048})
	again, e := reuse.Ensure(f.ctx, bodies)
	if e != nil || len(again) != len(refs) {
		t.Fatal(again, e)
	}
	g, h, p = f.remote.counts()
	if g != 6 || h != 3 || p != 3 {
		t.Fatal("warm reuse must not GET or PUT", g, h, p)
	}
	rows, e := c.LookupReceipts(f.ctx, ns, []objectstore.Ref{refs[1], refs[0], refs[1]}, 3)
	if e != nil || len(rows) != 3 || !rows[0].Found() || rows[0].ETag() != rows[2].ETag() {
		t.Fatal(rows, e)
	}
	for i, r := range refs {
		got, e := reuse.Read(f.ctx, r)
		if e != nil || !bytes.Equal(got, bodies[i]) {
			t.Fatal(got, e)
		}
	}
	if e = c.CreateReceiptSchema(f.ctx); e == nil {
		t.Fatal("schema must not auto migrate")
	}
	if _, e = c.BindNamespace(f.ctx, "alias", f.objects.Binding()); e == nil {
		t.Fatal("physical namespace alias accepted")
	}
}
func TestBoundsErrorsAndForgedProofs(t *testing.T) {
	f := setup(t)
	for _, b := range [][][]byte{{nil, nil, nil, nil, nil}, {make([]byte, 1025)}, {make([]byte, 1024), make([]byte, 1024), []byte("x")}} {
		r, e := f.reuse.Ensure(f.ctx, b)
		if e == nil || r != nil {
			t.Fatal(r, e)
		}
	}
	if g, h, p := f.remote.counts(); g+h+p != 0 {
		t.Fatal("admission did remote work")
	}
	if r, e := f.reuse.Ensure(context.Background(), nil); e == nil || r != nil {
		t.Fatal(r, e)
	}
	if e := f.cat.RecordVerified(f.ctx, f.ns, []objectstore.Verified{{}}, 4); e == nil {
		t.Fatal("forged proof accepted")
	}
	if r, e := f.cat.LookupReceipts(f.ctx, f.ns, []objectstore.Ref{{Bytes: -1}}, 4); e == nil || r != nil {
		t.Fatal(r, e)
	}
	if r, e := f.cat.LookupReceipts(f.ctx, f.ns, []objectstore.Ref{{}, {}, {}}, 2); e == nil || r != nil {
		t.Fatal(r, e)
	}
	refs, e := f.reuse.Ensure(f.ctx, [][]byte{[]byte("alpha")})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.pool.Exec(f.ctx, `UPDATE concordance_object_receipts SET etag='xx'`); e != nil {
		t.Fatal(e)
	}
	if r, e := f.cat.LookupReceipts(f.ctx, f.ns, refs, 4); e == nil || r != nil {
		t.Fatal("malformed row became miss", r, e)
	}
	f.pool.Close()
	if r, e := f.reuse.Ensure(f.ctx, [][]byte{[]byte("alpha")}); e == nil || r != nil {
		t.Fatal("database failure became miss", r, e)
	}
}
func TestKnownChangeQuarantinesWithoutHealingAndFencesPublication(t *testing.T) {
	f := setup(t)
	refs, e := f.reuse.Ensure(f.ctx, [][]byte{[]byte("alpha")})
	if e != nil {
		t.Fatal(e)
	}
	proof, e := f.objects.EnsureVerified(f.ctx, []byte("alpha"))
	if e != nil {
		t.Fatal(e)
	}
	req := f.request("before")
	if _, e = f.cat.Bind(f.ctx, req); e != nil {
		t.Fatal(e)
	}
	if _, e = f.cat.PublishInNamespace(f.ctx, req, candidate(), f.ns); e != nil {
		t.Fatal(e)
	}
	key, _ := f.objects.Key(refs[0])
	f.remote.mu.Lock()
	f.remote.objects["/bucket/"+key] = []byte("wrong")
	f.remote.mu.Unlock()
	_, _, puts := f.remote.counts()
	if r, e := f.reuse.Ensure(f.ctx, [][]byte{[]byte("alpha")}); !errors.Is(e, objectstore.ErrChanged) || r != nil {
		t.Fatal(r, e)
	}
	if _, _, p := f.remote.counts(); p != puts {
		t.Fatal("known change was repaired")
	}
	if e = f.cat.RecordVerified(f.ctx, f.ns, []objectstore.Verified{proof}, 4); !errors.Is(e, catalog.ErrNamespace) {
		t.Fatal("old proof retagged", e)
	}
	next := f.request("after")
	_, e = f.cat.Bind(f.ctx, next)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.cat.PublishInNamespace(f.ctx, next, candidate(), f.ns); !errors.Is(e, catalog.ErrNamespace) {
		t.Fatal("stale publication allowed", e)
	}
	result, e := f.cat.PublishInNamespace(f.ctx, req, candidate(), f.ns)
	if e != nil || !result.Duplicate {
		t.Fatal("original result lost", result, e)
	}
	if _, e = f.cat.BindNamespace(f.ctx, "fixture", f.objects.Binding()); !errors.Is(e, catalog.ErrNamespace) {
		t.Fatal("reactivated automatically", e)
	}
	if e = f.cat.Quarantine(f.ctx, f.ns); e != nil {
		t.Fatal("quarantine must be idempotent", e)
	}
}
func TestInvalidationDuringHeadRejectsCompletedBatch(t *testing.T) {
	f := setup(t)
	if _, e := f.reuse.Ensure(f.ctx, [][]byte{[]byte("alpha")}); e != nil {
		t.Fatal(e)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(release) }) })
	f.remote.headHook = func() {
		close(entered)
		select {
		case <-release:
		case <-f.ctx.Done():
		}
	}
	done := make(chan error, 1)
	go func() {
		r, e := f.reuse.Ensure(f.ctx, [][]byte{[]byte("alpha")})
		if r != nil {
			done <- errors.New("partial result escaped")
			return
		}
		done <- e
	}()
	select {
	case <-entered:
	case <-f.ctx.Done():
		t.Fatal(f.ctx.Err())
	}
	if e := f.cat.Quarantine(f.ctx, f.ns); e != nil {
		t.Fatal(e)
	}
	once.Do(func() { close(release) })
	if e := <-done; !errors.Is(e, catalog.ErrNamespace) {
		t.Fatal(e)
	}
}
func TestConcurrentCompatibleProofsAndConflictRollback(t *testing.T) {
	f := setup(t)
	proof, e := f.objects.EnsureVerified(f.ctx, []byte("alpha"))
	if e != nil {
		t.Fatal(e)
	}
	start := make(chan struct{})
	done := make(chan error, 8)
	for range 8 {
		go func() { <-start; done <- f.cat.RecordVerified(f.ctx, f.ns, []objectstore.Verified{proof, proof}, 4) }()
	}
	close(start)
	for range 8 {
		if e := <-done; e != nil {
			t.Fatal(e)
		}
	}
	ref, _ := proof.Reference()
	if _, e = f.pool.Exec(f.ctx, `UPDATE concordance_object_receipts SET etag='"different"'`); e != nil {
		t.Fatal(e)
	}
	if e = f.cat.RecordVerified(f.ctx, f.ns, []objectstore.Verified{proof}, 4); !errors.Is(e, catalog.ErrReceiptConflict) {
		t.Fatal(e)
	}
	rows, e := f.cat.LookupReceipts(f.ctx, f.ns, []objectstore.Ref{ref}, 4)
	if e != nil || rows[0].ETag() != `"different"` {
		t.Fatal("conflict overwrote evidence", rows, e)
	}
}

func TestWaitingVerifierCannotCommitAfterInvalidation(t *testing.T) {
	f := setup(t)
	proof, e := f.objects.EnsureVerified(f.ctx, []byte("alpha"))
	if e != nil {
		t.Fatal(e)
	}
	tx, e := f.pool.Begin(f.ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(f.ctx)
	if _, e = tx.Exec(f.ctx, `SELECT namespace_id FROM concordance_object_namespaces WHERE namespace_id='fixture' FOR UPDATE`); e != nil {
		t.Fatal(e)
	}
	done := make(chan error, 1)
	go func() { done <- f.cat.RecordVerified(f.ctx, f.ns, []objectstore.Verified{proof}, 4) }()
	// Observe the actual PostgreSQL lock wait; elapsed time is not synchronization.
	for {
		var waiting bool
		e = f.pool.QueryRow(f.ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE application_name=$1 AND wait_event_type='Lock')`, f.config.ConnConfig.RuntimeParams["application_name"]).Scan(&waiting)
		if e != nil {
			t.Fatal(e)
		}
		if waiting {
			break
		}
		if e = f.ctx.Err(); e != nil {
			t.Fatal(e)
		}
		runtime.Gosched()
	}
	if _, e = tx.Exec(f.ctx, `UPDATE concordance_object_namespaces SET active=false,epoch=epoch+1 WHERE namespace_id='fixture'`); e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(f.ctx); e != nil {
		t.Fatal(e)
	}
	if e = <-done; !errors.Is(e, catalog.ErrNamespace) {
		t.Fatal("stale verifier crossed epoch", e)
	}
	var count int
	if e = f.pool.QueryRow(f.ctx, `SELECT count(*) FROM concordance_object_receipts`).Scan(&count); e != nil || count != 0 {
		t.Fatal(count, e)
	}
}
func TestReadCorruptionFailsAndDisablesNewReuse(t *testing.T) {
	f := setup(t)
	refs, e := f.reuse.Ensure(f.ctx, [][]byte{[]byte("alpha")})
	if e != nil {
		t.Fatal(e)
	}
	key, _ := f.objects.Key(refs[0])
	f.remote.mu.Lock()
	f.remote.objects["/bucket/"+key] = []byte("wrong")
	f.remote.mu.Unlock()
	if b, e := f.reuse.Read(f.ctx, refs[0]); b != nil || !errors.Is(e, objectstore.ErrIntegrity) {
		t.Fatal(b, e)
	}
	if e = f.cat.CheckNamespace(f.ctx, f.ns); !errors.Is(e, catalog.ErrNamespace) {
		t.Fatal(e)
	}
}
func TestMissingReceiptReverifiesAndCanceledBatchHasNoOutput(t *testing.T) {
	f := setup(t)
	bodies := [][]byte{[]byte("alpha")}
	if _, e := f.reuse.Ensure(f.ctx, bodies); e != nil {
		t.Fatal(e)
	}
	if _, e := f.pool.Exec(f.ctx, `DELETE FROM concordance_object_receipts`); e != nil {
		t.Fatal(e)
	}
	if _, e := f.reuse.Ensure(f.ctx, bodies); e != nil {
		t.Fatal(e)
	}
	g, h, p := f.remote.counts()
	if g != 3 || h != 0 || p != 1 {
		t.Fatal("receipt loss must reverify existing bytes", g, h, p)
	}
	c, cancel := context.WithCancel(f.ctx)
	cancel()
	if r, e := f.reuse.Ensure(c, bodies); r != nil || !errors.Is(e, context.Canceled) {
		t.Fatal(r, e)
	}
}

func TestUnknownReadDoesNotQuarantineButKnownMissingReadDoes(t *testing.T) {
	f := setup(t)
	unknown := objectstore.Ref{SHA256: sha256.Sum256([]byte("never written")), Bytes: 13}
	if b, e := f.reuse.Read(f.ctx, unknown); b != nil || !errors.Is(e, objectstore.ErrNotFound) {
		t.Fatal(b, e)
	}
	if e := f.cat.CheckNamespace(f.ctx, f.ns); e != nil {
		t.Fatal("unknown read disabled namespace", e)
	}
	refs, e := f.reuse.Ensure(f.ctx, [][]byte{[]byte("alpha")})
	if e != nil {
		t.Fatal(e)
	}
	wrongSize := refs[0]
	wrongSize.Bytes++
	if b, e := f.reuse.Read(f.ctx, wrongSize); b != nil || e == nil {
		t.Fatal(b, e)
	}
	if e = f.cat.CheckNamespace(f.ctx, f.ns); e != nil {
		t.Fatal("caller size mismatch disabled namespace", e)
	}
	key, _ := f.objects.Key(refs[0])
	f.remote.mu.Lock()
	delete(f.remote.objects, "/bucket/"+key)
	f.remote.mu.Unlock()
	if b, e := f.reuse.Read(f.ctx, refs[0]); b != nil || !errors.Is(e, objectstore.ErrNotFound) {
		t.Fatal(b, e)
	}
	if e = f.cat.CheckNamespace(f.ctx, f.ns); !errors.Is(e, catalog.ErrNamespace) {
		t.Fatal("known missing body did not quarantine", e)
	}
}
