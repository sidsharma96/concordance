package objectstore

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go/logging"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type readProbe struct {
	reader    io.Reader
	readErr   error
	closeErr  error
	bytesRead int
	closed    bool
}

func (p *readProbe) Read(data []byte) (int, error) {
	if p.readErr != nil {
		return 0, p.readErr
	}
	n, err := p.reader.Read(data)
	p.bytesRead += n
	return n, err
}

func (p *readProbe) Close() error { p.closed = true; return p.closeErr }

func TestReadBoundsIntegrityAndCloseFailures(t *testing.T) {
	f := newFixture(t, nil)
	readFailure, closeFailure := errors.New("private read payload"), errors.New("private close payload")
	for _, tc := range []struct {
		name, body string
		advertised int
		readErr    error
		closeErr   error
		want       error
		maxRead    int
	}{
		{name: "good", body: "right", maxRead: 5},
		{name: "wrong hash", body: "wrong", want: ErrIntegrity, maxRead: 5},
		{name: "truncated", body: "righ", want: ErrIntegrity, maxRead: 4},
		{name: "excess", body: strings.Repeat("x", 1000), want: ErrIntegrity, maxRead: 6},
		{name: "wrong header", body: "right", advertised: 6, want: ErrIntegrity},
		{name: "read error", readErr: readFailure, want: readFailure},
		{name: "close error", body: "right", closeErr: closeFailure, want: closeFailure, maxRead: 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			probe := &readProbe{reader: strings.NewReader(tc.body), readErr: tc.readErr, closeErr: tc.closeErr}
			client := s3.New(s3.Options{
				Region: "auto", BaseEndpoint: aws.String("https://fixture.invalid"), UsePathStyle: true,
				Credentials:   aws.AnonymousCredentials{},
				ClientLogMode: aws.LogRequestWithBody | aws.LogResponseWithBody,
				Logger: logging.LoggerFunc(func(logging.Classification, string, ...interface{}) {
					t.Error("object operations must disable SDK logging")
				}),
				HTTPClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					header := make(http.Header)
					if tc.advertised != 0 {
						header.Set("Content-Length", strconv.Itoa(tc.advertised))
					}
					return &http.Response{StatusCode: 200, Header: header, Body: probe, ContentLength: -1, Request: r}, nil
				})},
			})
			store, err := New(client, "bucket", "collection", 1024)
			if err != nil {
				t.Fatal(err)
			}
			body, err := store.Read(f.ctx, reference([]byte("right")))
			if tc.want == nil {
				if err != nil || string(body) != "right" {
					t.Fatalf("good read: %q, %v", body, err)
				}
			} else if !errors.Is(err, tc.want) || body != nil || strings.Contains(err.Error(), "private") {
				t.Fatalf("failed read: %q, %v", body, err)
			}
			if !probe.closed || probe.bytesRead > tc.maxRead {
				t.Fatalf("body lifecycle/bound: closed=%v read=%d max=%d", probe.closed, probe.bytesRead, tc.maxRead)
			}
		})
	}
}

func TestScopeEncodingDoesNotCollapseDifferentIdentifiers(t *testing.T) {
	f := newFixture(t, nil)
	keys := make(map[string]bool)
	for _, id := range []string{"a/b", "a%2fb", "a/../b", "b", "tenant-a:guide", "tenant-b:guide", "文档"} {
		store, err := New(f.client, "bucket", id, 1024)
		if err != nil {
			t.Fatal(err)
		}
		key, err := store.Key(reference(nil))
		if err != nil || keys[key] {
			t.Fatalf("scope collision: %q, %v", key, err)
		}
		keys[key] = true
	}
	ctx, cancel := context.WithCancel(f.ctx)
	cancel()
	if _, err := f.store.Read(ctx, reference(nil)); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled read:", err)
	}
}

func TestSDKErrorBodiesAreBoundedBeforeDecoding(t *testing.T) {
	f := newFixture(t, nil)
	probe := &readProbe{reader: strings.NewReader(
		"<Error><Code>NoSuchKey</Code><Message>private</Message></Error>" + strings.Repeat("x", int(controlBodyLimit)),
	)}
	client := s3.New(s3.Options{
		Region: "auto", BaseEndpoint: aws.String("https://fixture.invalid"), UsePathStyle: true,
		Credentials: aws.AnonymousCredentials{},
		HTTPClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 404, Header: http.Header{"Content-Type": {"application/xml"}},
				Body: probe, ContentLength: -1, Request: r}, nil
		})},
	})
	store, err := New(client, "bucket", "collection", 1024)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Ensure(f.ctx, []byte("guide")); !errors.Is(err, ErrResponseLimit) || errors.Is(err, ErrNotFound) || strings.Contains(err.Error(), "private") {
		t.Fatal("oversized SDK error response:", err)
	}
	if !probe.closed || probe.bytesRead > int(controlBodyLimit)+1 {
		t.Fatalf("SDK error body unbounded/unclosed: closed=%v bytes=%d", probe.closed, probe.bytesRead)
	}
}
