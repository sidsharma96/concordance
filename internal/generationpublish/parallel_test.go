package generationpublish

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/sidsharma96/concordance/internal/catalog"
)

func parallelPublisher(t *testing.T, f storageFixture) *Publisher {
	t.Helper()
	l := testLimits()
	l.MaxBatchObjects = 32
	l.MaxConcurrentObjects = 8
	p, err := New(f.cat, f.objects, f.ns, l)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestParallelArtifactsFinishBeforeRootAndCompletedRetry(t *testing.T) {
	f := setup(t)
	b := makeBundle(t)
	p := parallelPublisher(t, f)
	root := f.rootPath(t, b)
	entered, gate := make(chan struct{}, len(b.artifacts)), make(chan struct{})
	var once sync.Once
	release := func() { once.Do(func() { close(gate) }) }
	t.Cleanup(release)
	var rootSeen atomic.Bool
	f.remote.setHook(func(r *http.Request) int {
		if r.URL.Path == root {
			rootSeen.Store(true)
			if f.remote.size() < len(b.artifacts) {
				t.Error("root started before all artifacts existed")
				return 500
			}
			return 0
		}
		if r.Method == "GET" {
			select {
			case entered <- struct{}{}:
			default:
			}
			select {
			case <-gate:
			case <-r.Context().Done():
				return 503
			}
		}
		return 0
	})
	out := launch(t, f, p, b, "parallel")
	for range len(b.artifacts) {
		select {
		case <-entered:
		case <-f.ctx.Done():
			t.Fatal(f.ctx.Err())
		}
	}
	if rootSeen.Load() {
		t.Fatal("root started while artifact workers blocked")
	}
	f.unpublished(t, "parallel")
	release()
	got := receive(t, f.ctx, out)
	if got.err != nil || !rootSeen.Load() || got.publication.Duplicate {
		t.Fatal(got, rootSeen.Load())
	}
	before := f.remote.counts()
	again, err := p.Publish(f.ctx, trustedRequest("parallel"), b.d.Generation, b.manifest, b.artifacts)
	if err != nil || !again.Duplicate || again.Generation != got.publication.Generation || f.remote.counts() != before {
		t.Fatal("completed retry used S3 or changed generation", again, err)
	}
}

func TestParallelArtifactFailureSkipsRootAndCanRetrySameKey(t *testing.T) {
	f := setup(t)
	b := makeBundle(t)
	p := parallelPublisher(t, f)
	root := f.rootPath(t, b)
	var rootSeen atomic.Bool
	f.remote.setHook(func(r *http.Request) int {
		if r.URL.Path == root {
			rootSeen.Store(true)
		}
		return 503
	})
	got, err := p.Publish(f.ctx, trustedRequest("parallel-failed"), b.d.Generation, b.manifest, b.artifacts)
	if err == nil || errors.Is(err, context.Canceled) || got != (catalog.Publication{}) || rootSeen.Load() {
		t.Fatal("failed artifact batch reached root/publication or reported peer cancellation", got, err, rootSeen.Load())
	}
	f.unpublished(t, "parallel-failed")
	f.remote.setHook(nil)
	got, err = p.Publish(f.ctx, trustedRequest("parallel-failed"), b.d.Generation, b.manifest, b.artifacts)
	if err != nil || got.Duplicate {
		t.Fatal("same-key retry did not publish", got, err)
	}
}
