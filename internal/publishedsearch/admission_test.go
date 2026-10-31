package publishedsearch

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/sidsharma96/concordance/internal/catalog"
	gm "github.com/sidsharma96/concordance/internal/generationmanifest"
	"github.com/sidsharma96/concordance/internal/objectstore"
)

func TestReaderInvalidConfigurationPrecedesLookup(t *testing.T) {
	f := setup(t)
	// Closed pool makes any accidental lookup fail differently. Invalid input must
	// be rejected independently of database availability, before any object request.
	f.pool.Close()
	selected := Selection{"guide", "generation-a", "one"}
	original := testLimits()
	for group := range reflect.TypeFor[Limits]().NumField() {
		v := reflect.ValueOf(original).Field(group)
		for field := range v.NumField() {
			for _, bad := range []int64{0, -1} {
				l := original
				reflect.ValueOf(&l).Elem().Field(group).Field(field).SetInt(bad)
				r, err := Open(testContext(t), f.cat, f.objects, selected, l)
				if !errors.Is(err, ErrInvalid) || r != nil {
					t.Fatalf("%s.%s=%d: %v", reflect.TypeFor[Limits]().Field(group).Name, v.Type().Field(field).Name, bad, err)
				}
			}
		}
	}
	for _, s := range []Selection{{"", selected.Generation, "one"}, {"guide", strings.Repeat("g", 257), "one"}, {"guide", "bad\x00id", "one"}, {"guide", selected.Generation, ""}, {"guide", selected.Generation, "\xff"}} {
		if r, err := Open(testContext(t), f.cat, f.objects, s, original); !errors.Is(err, ErrInvalid) || r != nil {
			t.Fatal("invalid selection reached lookup", err)
		}
	}
	for _, ctx := range []context.Context{nil, context.Background()} {
		if r, err := Open(ctx, f.cat, f.objects, selected, original); !errors.Is(err, ErrInvalid) || r != nil {
			t.Fatal("missing deadline", err)
		}
	}
	ctx, cancel := context.WithCancel(testContext(t))
	cancel()
	if r, err := Open(ctx, f.cat, f.objects, selected, original); !errors.Is(err, context.Canceled) || r != nil {
		t.Fatal("canceled open", err)
	}
	if r, err := Open(testContext(t), nil, f.objects, selected, original); !errors.Is(err, ErrInvalid) || r != nil {
		t.Fatal("nil catalog", err)
	}
	if r, err := Open(testContext(t), f.cat, nil, selected, original); !errors.Is(err, ErrInvalid) || r != nil {
		t.Fatal("nil object store", err)
	}
	if len(f.remote.counts()) != 0 {
		t.Fatal("invalid configuration caused I/O")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestReaderCanceledRootReadExposesNoState(t *testing.T) {
	f := setup(t)
	f.publish(t, makeBundle(t, "generation-a", "Retry uploads safely."))
	entered := make(chan struct{})
	client := s3.New(s3.Options{Region: "auto", BaseEndpoint: aws.String(f.server.URL), UsePathStyle: true, Credentials: aws.AnonymousCredentials{}, HTTPClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		close(entered)
		<-r.Context().Done()
		return nil, r.Context().Err()
	})}})
	objects, err := objectstore.New(client, "fixture", "guide", 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(testContext(t))
	defer cancel()
	type outcome struct {
		r   *Reader
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		r, err := Open(ctx, f.cat, objects, Selection{"guide", "generation-a", "one"}, testLimits())
		done <- outcome{r, err}
	}()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("root read never entered")
	}
	cancel()
	select {
	case got := <-done:
		if got.r != nil || !errors.Is(got.err, context.Canceled) {
			t.Fatal("partial canceled reader", got.err)
		}
	case <-testContext(t).Done():
		t.Fatal("canceled open did not finish")
	}
}

// Publish a deliberately inconsistent trusted record to exercise the reader's
// independent boundaries. The normal generation publisher rejects these bundles.
func (f fixture) publishUnchecked(t testing.TB, b bundle) {
	t.Helper()
	ctx := testContext(t)
	for _, body := range b.artifacts {
		if _, err := f.objects.Ensure(ctx, body); err != nil {
			t.Fatal(err)
		}
	}
	root, err := f.objects.Ensure(ctx, b.manifest)
	if err != nil {
		t.Fatal(err)
	}
	key, err := f.objects.Key(root)
	if err != nil {
		t.Fatal(err)
	}
	request := catalog.Request{Collection: "guide", Key: "unchecked", FingerprintVersion: "synthetic/v1"}
	if _, err := f.cat.Bind(ctx, request); err != nil {
		t.Fatal(err)
	}
	if _, err := f.cat.Publish(ctx, request, catalog.Generation{ID: "generation-a", ManifestKey: key, ManifestSHA256: root.SHA256, ManifestBytes: root.Bytes}); err != nil {
		t.Fatal(err)
	}
}
func TestReaderChecksManifestAndIndexAssociations(t *testing.T) {
	for _, name := range []string{"collection", "generation", "index article binding"} {
		t.Run(name, func(t *testing.T) {
			f := setup(t)
			b := makeBundle(t, "generation-a", "Retry uploads safely.")
			switch name {
			case "collection":
				b.description.Collection = "another"
			case "generation":
				b.description.Generation = "another"
			case "index article binding":
				other := makeBundle(t, "other", "Inspect failures first.")
				b.description.Editions[0].Index = other.description.Editions[0].Index
				b.artifacts = append(b.artifacts, other.artifacts...)
			}
			b.encode(t)
			f.publishUnchecked(t, b)
			if r, err := Open(testContext(t), f.cat, f.objects, Selection{"guide", "generation-a", "one"}, testLimits()); !errors.Is(err, gm.ErrMismatch) || r != nil {
				t.Fatal("inconsistent catalog artifacts accepted", err)
			}
		})
	}
}
