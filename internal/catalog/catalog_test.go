package catalog

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fixture struct {
	store       *Store
	pool        *pgxpool.Pool
	dsn, schema string
	ctx         context.Context
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	dsn := os.Getenv("CONCORDANCE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set CONCORDANCE_TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal("configure test database:", err)
	}
	t.Cleanup(admin.Close)
	schema := "concordance_test_" + strings.ToLower(rand.Text())
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		t.Fatal("create isolated test schema:", err)
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := admin.Exec(cleanup, "DROP SCHEMA "+quoted+" CASCADE"); err != nil {
			t.Error("drop isolated test schema:", err)
		}
	})
	f := fixture{dsn: dsn, schema: schema, ctx: ctx}
	f.store, f.pool = f.reopen(t)
	if err := f.store.CreateSchema(ctx); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f fixture) reopen(t *testing.T) (*Store, *pgxpool.Pool) {
	t.Helper()
	config, err := pgxpool.ParseConfig(f.dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = pgx.Identifier{f.schema}.Sanitize()
	config.MaxConns = 4
	pool, err := pgxpool.NewWithConfig(f.ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	store, err := New(pool)
	if err != nil {
		t.Fatal(err)
	}
	return store, pool
}

func request() Request {
	return Request{Collection: "guide", Key: "import-42", FingerprintVersion: "fixture-v1",
		Fingerprint: sha256.Sum256([]byte("illustrative captured input"))}
}

func generation(id string) Generation {
	return Generation{ID: id, ManifestKey: "guide/manifests/" + id,
		ManifestSHA256: sha256.Sum256([]byte("illustrative manifest " + id)), ManifestBytes: 32}
}

func TestBindingPreservesInputsBeforePublication(t *testing.T) {
	f := newFixture(t)
	req := request()
	for range 2 {
		record, err := f.store.Bind(f.ctx, req)
		if err != nil || record.Request != req || record.Generation != nil {
			t.Fatalf("bind: %+v, %v", record, err)
		}
	}
	changed := req
	changed.Fingerprint[0] ^= 1
	if _, err := f.store.Bind(f.ctx, changed); !errors.Is(err, ErrConflict) {
		t.Fatalf("changed input should conflict: %v", err)
	}
	changed = req
	changed.FingerprintVersion = "fixture-v2"
	if _, err := f.store.Bind(f.ctx, changed); !errors.Is(err, ErrFingerprintVersion) || errors.Is(err, ErrConflict) {
		t.Fatalf("different encoding cannot establish changed inputs: %v", err)
	}
	if err := f.store.CreateSchema(f.ctx); err == nil {
		t.Fatal("existing schema must not be silently replaced")
	}
	record, err := f.store.Lookup(f.ctx, req.Collection, req.Key)
	if err != nil || record.Request != req || record.Generation != nil {
		t.Fatalf("original binding changed: %+v, %v", record, err)
	}
	// Keys are opaque SQL parameters, not SQL or object paths.
	quoted := req
	quoted.Key = "'; DROP TABLE concordance_imports; --"
	if _, err := f.store.Bind(f.ctx, quoted); err != nil {
		t.Fatal(err)
	}
	otherCollection := req
	otherCollection.Collection = "other-guide"
	if _, err := f.store.Bind(f.ctx, otherCollection); err != nil {
		t.Fatal("keys must be collection-scoped:", err)
	}
}

func TestPublishedRetrySurvivesReconnection(t *testing.T) {
	f := newFixture(t)
	req := request()
	if _, err := f.store.Bind(f.ctx, req); err != nil {
		t.Fatal(err)
	}
	want := generation("generation-17")
	first, err := f.store.Publish(f.ctx, req, want)
	if err != nil || first.Duplicate || first.Generation != want {
		t.Fatalf("first publication: %+v, %v", first, err)
	}
	// Discard the caller's result and open another pool. This models a lost final
	// caller reply, not interruption of PostgreSQL's COMMIT acknowledgment.
	f.pool.Close()
	reopened, _ := f.reopen(t)
	duplicate, err := reopened.Publish(f.ctx, req, generation("unused-candidate"))
	if err != nil || !duplicate.Duplicate || duplicate.Generation != want {
		t.Fatalf("retry did not recover original: %+v, %v", duplicate, err)
	}
	record, err := reopened.Lookup(f.ctx, req.Collection, req.Key)
	if err != nil || record.Generation == nil || *record.Generation != want {
		t.Fatalf("reopen lookup: %+v, %v", record, err)
	}
	record.Generation.ManifestKey = "caller-modified-copy"
	got, err := reopened.LookupGeneration(f.ctx, req.Collection, want.ID)
	if err != nil || got != want {
		t.Fatalf("generation lookup/caller ownership: %+v, %v", got, err)
	}
	changed := req
	changed.Fingerprint[0] ^= 1
	if _, err := reopened.Publish(f.ctx, changed, generation("changed")); !errors.Is(err, ErrConflict) {
		t.Fatalf("published original must survive changed retry: %v", err)
	}
	newOperation := req
	newOperation.Key = "import-43"
	if _, err := reopened.Bind(f.ctx, newOperation); err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.Publish(f.ctx, newOperation, want); !errors.Is(err, ErrGenerationExists) {
		t.Fatalf("two keys cannot claim one generation: %v", err)
	}
	next := generation("generation-18")
	if result, err := reopened.Publish(f.ctx, newOperation, next); err != nil || result.Duplicate || result.Generation != next {
		t.Fatalf("new key expresses a new operation: %+v, %v", result, err)
	}
}

func TestConcurrentIdenticalPublicationsChooseOneResult(t *testing.T) {
	f := newFixture(t)
	req := request()
	if _, err := f.store.Bind(f.ctx, req); err != nil {
		t.Fatal(err)
	}
	const count = 16
	start := make(chan struct{})
	results := make(chan Publication, count)
	failures := make(chan error, count)
	var workers sync.WaitGroup
	for i := range count {
		workers.Go(func() {
			<-start
			result, err := f.store.Publish(f.ctx, req, generation(fmt.Sprintf("candidate-%d", i)))
			if err != nil {
				failures <- err
				return
			}
			results <- result
		})
	}
	close(start)
	workers.Wait()
	close(results)
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	var winner Generation
	newResults, observed := 0, 0
	for result := range results {
		if observed == 0 {
			winner = result.Generation
		}
		observed++
		if result.Generation != winner {
			t.Errorf("different results for one key: %+v and %+v", winner, result.Generation)
		}
		if !result.Duplicate {
			newResults++
		}
	}
	if observed != count || newResults != 1 {
		t.Fatalf("observed %d results, %d new; want %d and 1", observed, newResults, count)
	}
}

func TestConcurrentDifferentBindingsChooseOneInput(t *testing.T) {
	f := newFixture(t)
	a, b := request(), request()
	b.Fingerprint[0] ^= 1
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, req := range []Request{a, b} {
		go func() {
			<-start
			_, err := f.store.Bind(f.ctx, req)
			results <- err
		}()
	}
	close(start)
	success, conflict := 0, 0
	for range 2 {
		err := <-results
		switch {
		case err == nil:
			success++
		case errors.Is(err, ErrConflict):
			conflict++
		default:
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("success=%d conflict=%d", success, conflict)
	}
}

func TestIncompleteAbsentInvalidAndUnavailableAreDistinct(t *testing.T) {
	f := newFixture(t)
	req := request()
	if _, err := f.store.Publish(f.ctx, req, generation("unbound")); !errors.Is(err, ErrNotFound) {
		t.Fatalf("publication requires a binding: %v", err)
	}
	for _, invalid := range []Request{
		{Collection: req.Collection, FingerprintVersion: req.FingerprintVersion},
		{Collection: "\x00", Key: req.Key, FingerprintVersion: req.FingerprintVersion},
		{Collection: req.Collection, Key: strings.Repeat("x", 257), FingerprintVersion: req.FingerprintVersion},
		{Collection: req.Collection, Key: string([]byte{0xff}), FingerprintVersion: req.FingerprintVersion},
	} {
		if _, err := f.store.Bind(f.ctx, invalid); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid identifiers: %v", err)
		}
	}
	canceled, cancel := context.WithCancel(f.ctx)
	cancel()
	if _, err := f.store.Bind(canceled, req); !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-canceled binding: %v", err)
	}
	if _, err := f.store.Lookup(f.ctx, req.Collection, req.Key); !errors.Is(err, ErrNotFound) {
		t.Fatalf("pre-canceled operation created a binding: %v", err)
	}
	if _, err := f.store.Bind(f.ctx, req); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.Publish(canceled, req, generation("canceled")); !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-canceled publication: %v", err)
	}
	// The SQL invariant must reject a partial generation reference independently
	// of Go validation, leaving the binding unpublished.
	if _, err := f.pool.Exec(f.ctx, `UPDATE concordance_imports SET generation_id='partial'`); err == nil {
		t.Fatal("database accepted a partial reference")
	}
	record, err := f.store.Lookup(f.ctx, req.Collection, req.Key)
	if err != nil || record.Generation != nil {
		t.Fatalf("failed publication changed binding: %+v, %v", record, err)
	}
	f.pool.Close()
	if _, err := f.store.Lookup(f.ctx, req.Collection, req.Key); err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("pool error must not masquerade as absence: %v", err)
	}
}
