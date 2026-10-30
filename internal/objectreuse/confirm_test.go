package objectreuse

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/sidsharma96/concordance/internal/catalog"
	"github.com/sidsharma96/concordance/internal/objectstore"
)

type testClient func(*http.Request) (*http.Response, error)

func (c testClient) Do(r *http.Request) (*http.Response, error) { return c(r) }

func controlledReuse(t *testing.T, f fixture, client s3.HTTPClient, workers int) *Store {
	t.Helper()
	b := f.objects.Binding()
	c := s3.New(s3.Options{Region: "auto", BaseEndpoint: aws.String(b.Endpoint), UsePathStyle: true, Credentials: aws.AnonymousCredentials{}, HTTPClient: client})
	o, err := objectstore.New(c, b.Bucket, b.Collection, 1024)
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(f.cat, o, f.ns, Limits{MaxObjects: 32, MaxInputBytes: 2048, MaxConcurrentObjects: workers})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

type batchResult struct {
	refs []objectstore.Ref
	err  error
}

// Each launched Ensure is canceled and joined even if a test assertion fails.
func startBatch(t *testing.T, ctx context.Context, s *Store, bodies [][]byte, unblock func()) (<-chan batchResult, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan batchResult, 1)
	joined := make(chan struct{})
	go func() {
		defer close(joined)
		refs, err := s.Ensure(ctx, bodies)
		done <- batchResult{refs, err}
	}()
	t.Cleanup(func() {
		cancel()
		unblock()
		<-joined
	})
	return done, cancel
}

func nextBatch(t *testing.T, ctx context.Context, done <-chan batchResult) batchResult {
	t.Helper()
	select {
	case result := <-done:
		return result
	case <-ctx.Done():
		t.Fatal("batch did not finish", ctx.Err())
		return batchResult{}
	}
}

func signal(t *testing.T, ctx context.Context, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-ctx.Done():
		t.Fatal("expected worker event", ctx.Err())
	}
}

func TestParallelColdWarmOrderAndCeiling(t *testing.T) {
	for _, workers := range []int{0, 1, 2, 8} {
		t.Run(strconv.Itoa(workers), func(t *testing.T) {
			f := setup(t)
			bodies := make([][]byte, 16)
			for i := range bodies {
				bodies[i] = fmt.Appendf(nil, "object-%02d", i)
			}
			bodies = append(bodies, bodies[3], nil, bodies[0])
			want := make([]objectstore.Ref, len(bodies))
			for i, body := range bodies {
				want[i], _ = f.objects.Reference(body)
			}
			for _, warm := range []bool{false, true} {
				gate := make(chan struct{})
				var once sync.Once
				release := func() { once.Do(func() { close(gate) }) }
				entered := make(chan struct{}, 64)
				var active, peak atomic.Int32
				client := testClient(func(q *http.Request) (*http.Response, error) {
					n := active.Add(1)
					defer active.Add(-1)
					for old := peak.Load(); n > old && !peak.CompareAndSwap(old, n); old = peak.Load() {
					}
					entered <- struct{}{}
					select {
					case <-gate:
						return http.DefaultClient.Do(q)
					case <-q.Context().Done():
						return nil, q.Context().Err()
					}
				})
				s := controlledReuse(t, f, client, workers)
				done, _ := startBatch(t, f.ctx, s, bodies, release)
				limit := max(1, workers)
				for range limit {
					signal(t, f.ctx, entered)
				}
				release()
				got := nextBatch(t, f.ctx, done)
				if got.err != nil || !reflect.DeepEqual(got.refs, want) || peak.Load() != int32(limit) || active.Load() != 0 {
					t.Fatal("order/worker bound", warm, got, peak.Load(), active.Load())
				}
			}
			if g, h, p := f.remote.counts(); g != 34 || h != 17 || p != 17 {
				t.Fatal("concurrency changed distinct cold/warm operations", g, h, p)
			}
			t.Logf("workers=%d cold GET=34 PUT=17; warm HEAD=17; order/duplicates/empty preserved", workers)
		})
	}
}

type heldBody struct {
	read  func([]byte) (int, error)
	close func() error
}

func (b heldBody) Read(p []byte) (int, error) { return b.read(p) }
func (b heldBody) Close() error               { return b.close() }

func response(q *http.Request, status int, b []byte) *http.Response {
	h := make(http.Header)
	h.Set("Content-Length", strconv.Itoa(len(b)))
	h.Set("ETag", `"test-validator"`)
	h.Set("Content-Type", "application/xml")
	return &http.Response{StatusCode: status, Header: h, Body: io.NopCloser(bytes.NewReader(b)), ContentLength: int64(len(b)), Request: q}
}

func TestPeerCancellationJoinsCloseAndQuarantinesWithLiveCaller(t *testing.T) {
	f := setup(t)
	bad := []byte("bad")
	badRef, _ := f.objects.Reference(bad)
	badKey, _ := f.objects.Key(badRef)
	peerRead, peerCanceled, closing, release := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	client := testClient(func(q *http.Request) (*http.Response, error) {
		if q.URL.Path == "/bucket/"+badKey {
			select {
			case <-peerRead:
				return response(q, 200, []byte("BAD")), nil
			case <-q.Context().Done():
				return nil, q.Context().Err()
			}
		}
		r := response(q, 200, []byte("peer"))
		r.Body = heldBody{read: func([]byte) (int, error) {
			close(peerRead)
			<-q.Context().Done()
			close(peerCanceled)
			return 0, q.Context().Err()
		}, close: func() error {
			close(closing)
			<-release
			return nil
		}}
		return r, nil
	})
	s := controlledReuse(t, f, client, 2)
	done, _ := startBatch(t, f.ctx, s, [][]byte{bad, []byte("peer")}, unblock)
	signal(t, f.ctx, peerCanceled)
	signal(t, f.ctx, closing)
	select {
	case got := <-done:
		t.Fatal("returned before response Close completed", got)
	default:
	}
	if err := f.cat.CheckNamespace(f.ctx, f.ns); err != nil {
		t.Fatal("quarantined before workers joined", err)
	}
	unblock()
	got := nextBatch(t, f.ctx, done)
	if got.refs != nil || !errors.Is(got.err, objectstore.ErrIntegrity) || errors.Is(got.err, context.Canceled) || f.ctx.Err() != nil {
		t.Fatal("lost integrity or misreported caller cancellation", got)
	}
	if err := f.cat.CheckNamespace(f.ctx, f.ns); !errors.Is(err, catalog.ErrNamespace) {
		t.Fatal("peer cancellation prevented quarantine", err)
	}
}

func TestParallelIntegrityPreservesUncertainWrite(t *testing.T) {
	f := setup(t)
	bad := []byte("bad")
	badRef, _ := f.objects.Reference(bad)
	badKey, _ := f.objects.Key(badRef)
	put := make(chan struct{})
	var stored atomic.Bool
	client := testClient(func(q *http.Request) (*http.Response, error) {
		if q.URL.Path == "/bucket/"+badKey {
			select {
			case <-put:
				return response(q, 200, []byte("BAD")), nil
			case <-q.Context().Done():
				return nil, q.Context().Err()
			}
		}
		if q.Method == "PUT" {
			stored.Store(true) // The remote effect precedes the lost response.
			close(put)
			<-q.Context().Done()
			return nil, q.Context().Err()
		}
		return response(q, 404, []byte("<Error><Code>NoSuchKey</Code></Error>")), nil
	})
	s := controlledReuse(t, f, client, 2)
	got, err := s.Ensure(f.ctx, [][]byte{bad, []byte("new")})
	if got != nil || !errors.Is(err, objectstore.ErrIntegrity) || !errors.Is(err, objectstore.ErrOutcomeUnknown) || !stored.Load() {
		t.Fatal("lost material peer error", got, err, stored.Load())
	}
	var count int
	if e := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM concordance_object_receipts").Scan(&count); e != nil || count != 0 {
		t.Fatal("failed batch saved proof", count, e)
	}
	if e := f.cat.CheckNamespace(f.ctx, f.ns); !errors.Is(e, catalog.ErrNamespace) {
		t.Fatal("mixed uncertainty prevented quarantine", e)
	}
}

func TestCallerCancellationJoinsAndSavesNoSuccessfulProof(t *testing.T) {
	f := setup(t)
	verified, peerRead, closing, release := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	good := []byte("good")
	ref, _ := f.objects.Reference(good)
	key, _ := f.objects.Key(ref)
	markerRef, _ := f.objects.Reference([]byte("marker"))
	markerKey, _ := f.objects.Key(markerRef)
	client := testClient(func(q *http.Request) (*http.Response, error) {
		if q.URL.Path == "/bucket/"+key {
			return response(q, 200, good), nil
		}
		if q.URL.Path == "/bucket/"+markerKey {
			// Admission of a third job proves the good object's entire chain
			// completed and left a proof slot before caller cancellation.
			close(verified)
			<-q.Context().Done()
			return nil, q.Context().Err()
		}
		r := response(q, 200, []byte("peer"))
		r.Body = heldBody{read: func([]byte) (int, error) {
			close(peerRead)
			<-q.Context().Done()
			return 0, q.Context().Err()
		}, close: func() error { close(closing); <-release; return nil }}
		return r, nil
	})
	s := controlledReuse(t, f, client, 2)
	done, cancel := startBatch(t, f.ctx, s, [][]byte{good, []byte("peer"), []byte("marker")}, unblock)
	signal(t, f.ctx, verified)
	signal(t, f.ctx, peerRead)
	cancel()
	signal(t, f.ctx, closing)
	select {
	case got := <-done:
		t.Fatal("caller cancellation returned before Close", got)
	default:
	}
	unblock()
	got := nextBatch(t, f.ctx, done)
	if got.refs != nil || !errors.Is(got.err, context.Canceled) {
		t.Fatal(got)
	}
	var count int
	if err := f.pool.QueryRow(f.ctx, "SELECT count(*) FROM concordance_object_receipts").Scan(&count); err != nil || count != 0 {
		t.Fatal("partial successful proof saved", count, err)
	}
	if err := f.cat.CheckNamespace(f.ctx, f.ns); err != nil {
		t.Fatal("ordinary cancellation quarantined namespace", err)
	}
}

func TestConcurrencyLimitAdmissionAndCancellationTrees(t *testing.T) {
	f := setup(t)
	for _, workers := range []int{-1, 9} {
		if s, err := New(f.cat, f.objects, f.ns, Limits{MaxObjects: 32, MaxInputBytes: 2048, MaxConcurrentObjects: workers}); s != nil || !errors.Is(err, ErrInvalid) {
			t.Fatal(workers, s, err)
		}
	}
	for _, tc := range []struct {
		err  error
		want bool
	}{
		{nil, false}, {context.Canceled, true}, {fmt.Errorf("wrapped: %w", context.Canceled), true},
		{errors.Join(context.Canceled, context.Canceled), true},
		{errors.Join(objectstore.ErrOutcomeUnknown, context.Canceled), false},
		{errors.Join(objectstore.ErrIntegrity, context.Canceled), false},
		{context.DeadlineExceeded, false},
	} {
		if got := cancellationOnly(tc.err); got != tc.want {
			t.Fatal(tc.err, got, tc.want)
		}
	}
	if g, h, p := f.remote.counts(); g+h+p != 0 {
		t.Fatal("invalid configuration did remote work", g, h, p)
	}
}

func TestIntegrityAndFailedQuarantineRemainObservable(t *testing.T) {
	f := setup(t)
	client := testClient(func(q *http.Request) (*http.Response, error) {
		// The receipt lookup has finished before object work starts. Closing the
		// pool now makes the later quarantine attempt fail, without canceling
		// the live caller or hiding the independently observed corrupt bytes.
		f.pool.Close()
		return response(q, 200, []byte("BAD")), nil
	})
	s := controlledReuse(t, f, client, 8)
	refs, err := s.Ensure(f.ctx, [][]byte{[]byte("bad")})
	if refs != nil || !errors.Is(err, objectstore.ErrIntegrity) || errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "quarantine namespace") {
		t.Fatal("failed quarantine or integrity cause hidden", refs, err)
	}
}

func TestTransportCancellationCannotBecomeUncheckedWarmSuccess(t *testing.T) {
	for _, workers := range []int{0, 8} {
		t.Run(strconv.Itoa(workers), func(t *testing.T) {
			f := setup(t)
			bodies := [][]byte{[]byte("one"), []byte("two"), []byte("three")}
			if _, err := f.reuse.Ensure(f.ctx, bodies); err != nil {
				t.Fatal(err)
			}
			var calls atomic.Int32
			s := controlledReuse(t, f, testClient(func(*http.Request) (*http.Response, error) {
				calls.Add(1)
				// The transport, not the caller or a peer, originates this error.
				return nil, context.Canceled
			}), workers)
			refs, err := s.Ensure(f.ctx, bodies)
			if refs != nil || !errors.Is(err, context.Canceled) || f.ctx.Err() != nil || calls.Load() == 0 {
				t.Fatal("transport cancellation became unchecked success", refs, err, f.ctx.Err(), calls.Load())
			}
			if err := f.cat.CheckNamespace(f.ctx, f.ns); err != nil {
				t.Fatal("transport cancellation disabled namespace", err)
			}
		})
	}
}
