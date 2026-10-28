package objectstore

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func TestVerifiedEvidenceAndMetadataCheck(t *testing.T) {
	f := newFixture(t, func(b *backend) {
		b.before = func(w http.ResponseWriter, r *http.Request, _ string, _ int) bool {
			if r.Method != "HEAD" {
				return false
			}
			if r.Header.Get("If-Match") != `"not-a-raw-content-checksum"` {
				t.Error("missing saved validator")
			}
			w.Header().Set("ETag", `"not-a-raw-content-checksum"`)
			w.Header().Set("Content-Length", "5")
			return true
		}
	})
	v, e := f.store.EnsureVerified(f.ctx, []byte("right"))
	if e != nil {
		t.Fatal(e)
	}
	evidence, e := v.Evidence()
	if e != nil || evidence.Ref != reference([]byte("right")) || evidence.Binding != f.store.Binding() {
		t.Fatal(evidence, e)
	}
	before, puts, _ := f.remote.counts()
	if e = f.store.Check(f.ctx, evidence.Ref, evidence.ETag); e != nil {
		t.Fatal(e)
	}
	after, nextPuts, _ := f.remote.counts()
	if after != before+1 || nextPuts != puts {
		t.Fatal("HEAD must be one operation and never PUT")
	}
	if _, e = (Verified{}).Evidence(); !errors.Is(e, ErrInvalid) {
		t.Fatal(e)
	}
	if _, e = f.store.Reference(make([]byte, 1025)); !errors.Is(e, ErrInvalid) {
		t.Fatal(e)
	}
}

func TestProofRequiresSuccessfulReadCloseAndSameResponseValidator(t *testing.T) {
	for _, tag := range []string{`"first-response"`, "", `W/"weak"`, "unquoted", `"bad` + "\n" + `tag"`, strings.Repeat("x", 1025)} {
		for _, badClose := range []bool{false, true} {
			t.Run(tag+string(rune('0'+len(tag)%10)), func(t *testing.T) {
				probe := &readProbe{reader: strings.NewReader("right")}
				if badClose {
					probe.closeErr = errors.New("close fault")
				}
				client := s3.New(s3.Options{Region: "auto", BaseEndpoint: aws.String("https://fixture.invalid"), Credentials: aws.AnonymousCredentials{}, HTTPClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					return &http.Response{StatusCode: 200, Header: http.Header{"Etag": {tag}}, Body: probe, ContentLength: -1, Request: r}, nil
				})}})
				s, _ := New(client, "bucket", "scope", 100)
				v, e := s.EnsureVerified(t.Context(), []byte("right"))
				// Workful object operations require a deadline.
				if e == nil {
					t.Fatal("missing deadline accepted")
				}
				ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
				defer cancel()
				v, e = s.EnsureVerified(ctx, []byte("right"))
				if badClose {
					if e == nil {
						t.Fatal("close failure produced proof")
					}
					if _, x := v.Evidence(); !errors.Is(x, ErrInvalid) {
						t.Fatal(x)
					}
					return
				}
				if e != nil {
					t.Fatal(e)
				}
				a, x := v.Evidence()
				if ValidETag(tag) {
					if x != nil || a.ETag != tag {
						t.Fatal(a, x)
					}
				} else if !errors.Is(x, ErrNoValidator) {
					t.Fatal(x)
				}
			})
		}
	}
}

func TestHeadFailuresDoNotAuthorizeCreate(t *testing.T) {
	f := newFixture(t, nil)
	for _, status := range []int{403, 404, 412, 500} {
		probe := &readProbe{reader: strings.NewReader("")}
		client := s3.New(s3.Options{Region: "auto", BaseEndpoint: aws.String("https://fixture.invalid"), Credentials: aws.AnonymousCredentials{}, HTTPClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.Method != "HEAD" {
				t.Error("unexpected non HEAD")
			}
			return &http.Response{StatusCode: status, Header: http.Header{}, Body: probe, Request: r}, nil
		})}})
		s, _ := New(client, "bucket", "scope", 100)
		e := s.Check(f.ctx, reference([]byte("right")), `"tag"`)
		if e == nil || errors.Is(e, ErrNotFound) {
			t.Fatal(status, e)
		}
		if (status == 404 || status == 412) != errors.Is(e, ErrChanged) {
			t.Fatal(status, e)
		}
	}
	canceled, cancel := context.WithCancel(f.ctx)
	cancel()
	if e := f.store.Check(canceled, reference(nil), `"tag"`); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}
