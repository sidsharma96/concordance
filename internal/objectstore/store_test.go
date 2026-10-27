package objectstore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
)

// This HTTP fixture exercises the real SDK's signing, headers, error decoding and
// retry options. It is deliberately not an emulator or evidence of R2 semantics.
type backend struct {
	mu       sync.Mutex
	objects  map[string][]byte
	requests int
	puts     int
	created  int
	before   func(http.ResponseWriter, *http.Request, string, int) bool
	afterPut func(http.ResponseWriter, *http.Request) bool
}

func (b *backend) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Path
	b.mu.Lock()
	b.requests++
	op := b.requests
	if r.Method == http.MethodPut {
		b.puts++
	}
	b.mu.Unlock()
	if b.before != nil && b.before(w, r, key, op) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		b.mu.Lock()
		body, exists := b.objects[key]
		body = bytes.Clone(body)
		b.mu.Unlock()
		if !exists {
			apiFailure(w, 404, "NoSuchKey")
			return
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.Header().Set("ETag", `"not-a-raw-content-checksum"`)
		_, _ = w.Write(body)
	case http.MethodPut:
		if r.Header.Get("If-None-Match") != "*" || r.Header.Get("Content-Type") != "application/octet-stream" {
			apiFailure(w, 400, "InvalidRequest")
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 64<<10))
		if err != nil {
			apiFailure(w, 400, "InvalidRequest")
			return
		}
		b.mu.Lock()
		_, exists := b.objects[key]
		if !exists {
			b.objects[key] = body
			b.created++
		}
		b.mu.Unlock()
		if exists {
			apiFailure(w, 412, "PreconditionFailed")
			return
		}
		if b.afterPut != nil && b.afterPut(w, r) {
			return
		}
		w.WriteHeader(200)
	default:
		apiFailure(w, 405, "MethodNotAllowed")
	}
}

func apiFailure(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, "<Error><Code>%s</Code><Message>payload-do-not-log</Message></Error>", code)
}

func disconnect(w http.ResponseWriter) {
	conn, _, err := w.(http.Hijacker).Hijack()
	if err == nil {
		_ = conn.Close()
	}
}

type fixture struct {
	store  *Store
	client *s3.Client
	ctx    context.Context
	remote *backend
}

func newFixture(t *testing.T, configure func(*backend)) fixture {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	b := &backend{objects: make(map[string][]byte)}
	if configure != nil {
		configure(b)
	}
	server := httptest.NewServer(b)
	t.Cleanup(server.Close)
	t.Cleanup(cancel) // Cancel request contexts before server.Close waits for handlers.
	client := s3.New(s3.Options{
		Region: "auto", BaseEndpoint: aws.String(server.URL), UsePathStyle: true,
		Credentials: aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
			return aws.Credentials{AccessKeyID: "synthetic-key", SecretAccessKey: "synthetic-secret", Source: "test"}, nil
		}), HTTPClient: server.Client(),
	})
	store, err := New(client, "fixture-bucket", "tenant-a/collection", 1024)
	if err != nil {
		t.Fatal(err)
	}
	return fixture{store: store, client: client, ctx: ctx, remote: b}
}

func reference(body []byte) Ref {
	return Ref{SHA256: sha256.Sum256(body), Bytes: int64(len(body))}
}

func (b *backend) counts() (requests, puts, created int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.requests, b.puts, b.created
}

func TestEnsureReusesVerifiedBytesAndSeparatesCollections(t *testing.T) {
	f := newFixture(t, nil)
	for _, body := range [][]byte{[]byte("same guide"), nil} {
		want := reference(body)
		for range 2 {
			ref, err := f.store.Ensure(f.ctx, body)
			if err != nil || ref != want {
				t.Fatalf("ensure: %+v, %v", ref, err)
			}
		}
		got, err := f.store.Read(f.ctx, want)
		if err != nil || !bytes.Equal(got, body) {
			t.Fatalf("read: %q, %v", got, err)
		}
		if len(got) > 0 {
			got[0] = 'X'
			again, err := f.store.Read(f.ctx, want)
			if err != nil || !bytes.Equal(again, body) {
				t.Fatal("caller changed remote bytes:", err)
			}
		}
	}
	if _, puts, created := f.remote.counts(); puts != 2 || created != 2 {
		t.Fatalf("two unique byte strings should create two objects: puts=%d created=%d", puts, created)
	}
	other, err := New(f.client, "fixture-bucket", "tenant-b/collection", 1024)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.Ensure(f.ctx, []byte("same guide")); err != nil {
		t.Fatal(err)
	}
	if _, puts, created := f.remote.counts(); puts != 3 || created != 3 {
		t.Fatalf("collections must not share objects: puts=%d created=%d", puts, created)
	}
	key, err := f.store.Key(reference([]byte("same guide")))
	encoded := hex.EncodeToString([]byte("tenant-a/collection"))
	if err != nil || !strings.HasPrefix(key, "v1/collections/"+encoded+"/sha256/") {
		t.Fatalf("scope encoding: %q, %v", key, err)
	}
}

func TestConcurrentCreatorsUseConditionalWrites(t *testing.T) {
	arrived, release := make(chan struct{}, 2), make(chan struct{})
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(release) }) })
	var gets atomic.Int32
	f := newFixture(t, func(b *backend) {
		b.before = func(w http.ResponseWriter, r *http.Request, _ string, _ int) bool {
			if r.Method == http.MethodGet && gets.Add(1) <= 2 {
				arrived <- struct{}{}
				select {
				case <-release:
					apiFailure(w, 404, "NoSuchKey")
				case <-r.Context().Done():
				}
				return true
			}
			return false
		}
	})
	results := make(chan error, 2)
	for range 2 {
		go func() {
			ref, err := f.store.Ensure(f.ctx, []byte("concurrent guide"))
			if err == nil && ref != reference([]byte("concurrent guide")) {
				err = errors.New("wrong reference")
			}
			results <- err
		}()
	}
	for range 2 {
		select {
		case <-arrived:
		case <-f.ctx.Done():
			t.Fatal("creators did not reach barrier:", f.ctx.Err())
		}
	}
	once.Do(func() { close(release) })
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	if requests, puts, created := f.remote.counts(); requests != 6 || puts != 2 || created != 1 {
		t.Fatalf("conditional race: requests=%d puts=%d created=%d", requests, puts, created)
	}
}

func TestLostCreateReplyIsResolvedByVerifiedRead(t *testing.T) {
	f := newFixture(t, func(b *backend) {
		b.afterPut = func(w http.ResponseWriter, _ *http.Request) bool { disconnect(w); return true }
	})
	body := []byte("committed before disconnect")
	if got, err := f.store.Ensure(f.ctx, body); err != nil || got != reference(body) {
		t.Fatalf("verified object should resolve lost reply: %+v, %v", got, err)
	}
	if requests, puts, created := f.remote.counts(); requests != 3 || puts != 1 || created != 1 {
		t.Fatalf("hidden retries: requests=%d puts=%d created=%d", requests, puts, created)
	}
}

func TestUncertainWriteIsNotObjectAbsence(t *testing.T) {
	f := newFixture(t, func(b *backend) {
		b.before = func(w http.ResponseWriter, r *http.Request, _ string, _ int) bool {
			if r.Method == http.MethodPut {
				disconnect(w)
				return true
			}
			return false
		}
	})
	if got, err := f.store.Ensure(f.ctx, []byte("not committed")); !errors.Is(err, ErrOutcomeUnknown) || errors.Is(err, ErrNotFound) || got != (Ref{}) {
		t.Fatalf("must remain unresolved, not absent: %+v, %v", got, err)
	}
	if requests, puts, _ := f.remote.counts(); requests != 3 || puts != 1 {
		t.Fatalf("unexpected retries: requests=%d puts=%d", requests, puts)
	}
}

func TestReadFailuresDoNotPermitOverwriteAndHideMessages(t *testing.T) {
	for _, code := range []string{"AccessDenied", "NoSuchBucket", "InternalError"} {
		t.Run(code, func(t *testing.T) {
			f := newFixture(t, func(b *backend) {
				b.before = func(w http.ResponseWriter, _ *http.Request, _ string, _ int) bool {
					status := 500
					if code == "AccessDenied" {
						status = 403
					} else if code == "NoSuchBucket" {
						status = 404
					}
					apiFailure(w, status, code)
					return true
				}
			})
			_, err := f.store.Ensure(f.ctx, []byte("guide"))
			var serviceError smithy.APIError
			if err == nil || errors.Is(err, ErrNotFound) || strings.Contains(err.Error(), "payload-do-not-log") || !errors.As(err, &serviceError) || serviceError.ErrorCode() != code {
				t.Fatalf("error classification/formatting: %v", err)
			}
			if requests, puts, _ := f.remote.counts(); requests != 1 || puts != 0 {
				t.Fatalf("read failure must not trigger put/retries: %d, %d", requests, puts)
			}
		})
	}
	f := newFixture(t, nil)
	ref := reference([]byte("right"))
	key, _ := f.store.Key(ref)
	f.remote.objects["/fixture-bucket/"+key] = []byte("wrong")
	if _, err := f.store.Ensure(f.ctx, []byte("right")); !errors.Is(err, ErrIntegrity) {
		t.Fatal("wrong existing bytes must fail:", err)
	}
	if _, puts, _ := f.remote.counts(); puts != 0 {
		t.Fatal("must never overwrite wrong content")
	}
}

func TestPostWriteVerificationAndCancellationRemainUnresolved(t *testing.T) {
	for _, failure := range []string{"unavailable", "canceled"} {
		t.Run(failure, func(t *testing.T) {
			var cancel context.CancelFunc
			f := newFixture(t, func(b *backend) {
				if failure == "unavailable" {
					b.before = func(w http.ResponseWriter, r *http.Request, _ string, op int) bool {
						if op == 3 && r.Method == http.MethodGet {
							apiFailure(w, 500, "InternalError")
							return true
						}
						return false
					}
				} else {
					b.afterPut = func(w http.ResponseWriter, _ *http.Request) bool { cancel(); disconnect(w); return true }
				}
			})
			ctx, stop := context.WithCancel(f.ctx)
			cancel = stop
			defer stop()
			body := []byte("durable fixture bytes")
			if _, err := f.store.Ensure(ctx, body); !errors.Is(err, ErrOutcomeUnknown) || failure == "canceled" && !errors.Is(err, context.Canceled) {
				t.Fatalf("unresolved %s: %v", failure, err)
			}
			// New context, same bytes: recover by read without uploading again.
			if got, err := f.store.Ensure(f.ctx, body); err != nil || got != reference(body) {
				t.Fatalf("retry failed: %+v, %v", got, err)
			}
			if _, puts, created := f.remote.counts(); puts != 1 || created != 1 {
				t.Fatalf("retry changed identity: puts=%d created=%d", puts, created)
			}
		})
	}
}

func TestValidationAndPreCancellationAvoidRequests(t *testing.T) {
	f := newFixture(t, nil)
	if _, err := f.store.Ensure(f.ctx, make([]byte, 1025)); !errors.Is(err, ErrInvalid) {
		t.Fatal("oversize input:", err)
	}
	if _, err := f.store.Read(f.ctx, Ref{Bytes: -1}); !errors.Is(err, ErrInvalid) {
		t.Fatal("negative size:", err)
	}
	if _, err := f.store.Read(f.ctx, Ref{Bytes: 1025}); !errors.Is(err, ErrInvalid) {
		t.Fatal("oversize reference:", err)
	}
	if _, err := f.store.Ensure(context.Background(), nil); !errors.Is(err, ErrInvalid) {
		t.Fatal("missing deadline:", err)
	}
	ctx, cancel := context.WithCancel(f.ctx)
	cancel()
	if _, err := f.store.Ensure(ctx, nil); !errors.Is(err, context.Canceled) {
		t.Fatal("pre-cancellation:", err)
	}
	if _, err := New(nil, "bucket", "collection", 1024); !errors.Is(err, ErrInvalid) {
		t.Fatal("nil client:", err)
	}
	if requests, _, _ := f.remote.counts(); requests != 0 {
		t.Fatal("invalid/canceled input reached backend")
	}
	if _, err := f.store.Read(f.ctx, reference([]byte("absent"))); !errors.Is(err, ErrNotFound) {
		t.Fatal("explicit object absence:", err)
	}
}

func TestWrongContentAfterCreateIsIntegrityFailure(t *testing.T) {
	f := newFixture(t, func(b *backend) {
		b.afterPut = func(_ http.ResponseWriter, r *http.Request) bool {
			b.mu.Lock()
			b.objects[r.URL.Path] = []byte("wrong")
			b.mu.Unlock()
			return false
		}
	})
	if _, err := f.store.Ensure(f.ctx, []byte("right")); !errors.Is(err, ErrIntegrity) || errors.Is(err, ErrOutcomeUnknown) {
		t.Fatal("verified mismatch must stay an integrity error:", err)
	}
}
