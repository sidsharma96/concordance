package generationpublish

import (
	"context"
	"errors"
	"net/http"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/sidsharma96/concordance/internal/catalog"
)

type outcome struct {
	publication catalog.Publication
	err         error
}

func launch(t *testing.T, f storageFixture, p *Publisher, b bundle, key string) <-chan outcome {
	t.Helper()
	ctx, cancel := context.WithCancel(f.ctx)
	out := make(chan outcome, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		result, err := p.Publish(ctx, trustedRequest(key), b.d.Generation, b.manifest, b.artifacts)
		out <- outcome{result, err}
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Error("publication caller did not finish")
		}
	})
	return out
}
func receive(t *testing.T, ctx context.Context, out <-chan outcome) outcome {
	t.Helper()
	select {
	case result := <-out:
		return result
	case <-ctx.Done():
		t.Fatal("publication did not complete", ctx.Err())
		return outcome{}
	}
}
func TestPermitRejectsWaitingCallersAndReleases(t *testing.T) {
	f := setup(t)
	b := makeBundle(t)
	entered := make(chan struct{})
	proceed := make(chan struct{})
	var enter, release sync.Once
	t.Cleanup(func() { release.Do(func() { close(proceed) }) })
	f.remote.setHook(func(r *http.Request) int {
		enter.Do(func() { close(entered) })
		select {
		case <-proceed:
			return 0
		case <-r.Context().Done():
			return 503
		}
	})
	out := launch(t, f, f.publisher, b, "first")
	select {
	case <-entered:
	case <-f.ctx.Done():
		t.Fatal(f.ctx.Err())
	}
	before := f.remote.counts()
	if got, err := runPublish(f, b, "another"); !errors.Is(err, ErrBusy) || got != (catalog.Publication{}) {
		t.Fatal(got, err)
	}
	if f.remote.counts() != before {
		t.Fatal("rejected caller performed HTTP work")
	}
	if _, err := f.cat.Lookup(f.ctx, "guide", "another"); !errors.Is(err, catalog.ErrNotFound) {
		t.Fatal("busy caller bound key", err)
	}
	release.Do(func() { close(proceed) })
	result := receive(t, f.ctx, out)
	if result.err != nil || result.publication.Duplicate {
		t.Fatal(result)
	}
	if got, err := runPublish(f, b, "first"); err != nil || !got.Duplicate {
		t.Fatal("permit not released", got, err)
	}
}
func TestIndependentPublishersChooseOneResult(t *testing.T) {
	f := setup(t)
	one := makeBundle(t)
	two := makeBundle(t)
	two.d.Generation = "generation-2"
	two.encode(t)
	other, err := New(f.cat, f.objects, f.ns, testLimits())
	if err != nil {
		t.Fatal(err)
	}
	// A common start barrier exercises different local permits; neither claims
	// distributed ownership of the key. The catalog determines the winner.
	start := make(chan struct{})
	outputs := make(chan outcome, 2)
	var wg sync.WaitGroup
	for _, call := range []struct {
		p *Publisher
		b bundle
	}{{f.publisher, one}, {other, two}} {
		wg.Go(func() {
			<-start
			result, err := call.p.Publish(f.ctx, trustedRequest("same"), call.b.d.Generation, call.b.manifest, call.b.artifacts)
			outputs <- outcome{result, err}
		})
	}
	close(start)
	wg.Wait()
	close(outputs)
	duplicates := 0
	var winner catalog.Generation
	for out := range outputs {
		if out.err != nil {
			t.Fatal(out.err)
		}
		if out.publication.Duplicate {
			duplicates++
		}
		if winner.ID == "" {
			winner = out.publication.Generation
		} else if winner != out.publication.Generation {
			t.Fatal("two publication results")
		}
	}
	if duplicates != 1 {
		t.Fatal("expected one winner and one duplicate", duplicates)
	}
	record, err := f.cat.Lookup(f.ctx, "guide", "same")
	if err != nil || record.Generation == nil || *record.Generation != winner {
		t.Fatal(record, err)
	}
}
func TestQuarantineAfterConfirmationFencesPublication(t *testing.T) {
	f := setup(t)
	b := makeBundle(t)
	rootPath := f.rootPath(t, b)
	locked := make(chan pgx.Tx, 1)
	var once sync.Once
	f.remote.setHook(func(r *http.Request) int {
		if r.Method != "GET" || r.URL.Path != rootPath {
			return 0
		}
		f.remote.mu.Lock()
		_, exists := f.remote.bodies[rootPath]
		f.remote.mu.Unlock()
		if !exists {
			return 0
		} // First GET is the miss; second verifies the stored root.
		status := 0
		once.Do(func() {
			tx, err := f.pool.Begin(f.ctx)
			if err != nil {
				t.Error(err)
				status = 500
				return
			}
			if _, err := tx.Exec(f.ctx, "SELECT import_key FROM concordance_imports WHERE collection_id=$1 AND import_key=$2 FOR UPDATE", "guide", "fenced"); err != nil {
				t.Error(err)
				_ = tx.Rollback(f.ctx)
				status = 500
				return
			}
			locked <- tx
		})
		return status
	})
	out := launch(t, f, f.publisher, b, "fenced")
	var lock pgx.Tx
	select {
	case lock = <-locked:
	case <-f.ctx.Done():
		t.Fatal(f.ctx.Err())
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := lock.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Error(err)
		}
	})
	// Wait for the publisher's SELECT FOR UPDATE to block behind our row lock.
	// Its remote confirmations have finished by this point. Observe database state,
	// not elapsed time, before invalidating the namespace.
	for {
		var waiting bool
		err := f.pool.QueryRow(f.ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity
   WHERE application_name=$1 AND wait_event_type='Lock' AND query LIKE '%FOR UPDATE%')`, f.config.ConnConfig.RuntimeParams["application_name"]).Scan(&waiting)
		if err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		runtime.Gosched()
	}
	n := len(b.artifacts) + 1
	if f.remote.size() != n || f.remote.counts() != [3]int{2 * n, 0, n} {
		t.Fatal("publication waited before confirmations finished")
	}
	if err := f.cat.Quarantine(f.ctx, f.ns); err != nil {
		t.Fatal(err)
	}
	if err := lock.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	result := receive(t, f.ctx, out)
	if !errors.Is(result.err, catalog.ErrNamespace) || result.publication != (catalog.Publication{}) {
		t.Fatal("namespace fence bypassed", result)
	}
	f.unpublished(t, "fenced")
}
func TestCancellationDoesNotPublishAndReleasesPermit(t *testing.T) {
	f := setup(t)
	b := makeBundle(t)
	ctx, cancel := context.WithCancel(f.ctx)
	defer cancel()
	f.remote.setHook(func(*http.Request) int { cancel(); return 503 })
	out, err := f.publisher.Publish(ctx, trustedRequest("cancelled"), b.d.Generation, b.manifest, b.artifacts)
	if !errors.Is(err, context.Canceled) || out != (catalog.Publication{}) {
		t.Fatal(out, err)
	}
	f.unpublished(t, "cancelled")
	f.remote.setHook(nil)
	if out, err := runPublish(f, b, "cancelled"); err != nil || out.Duplicate {
		t.Fatal("explicit retry after cancel", out, err)
	}
}
