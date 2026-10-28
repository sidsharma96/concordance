package htmlpassagemap

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/sidsharma96/concordance/internal/htmlarticle"
	"github.com/sidsharma96/concordance/internal/htmlarticlejson"
	"github.com/sidsharma96/concordance/internal/htmlpassage"
	"github.com/sidsharma96/concordance/internal/htmlsearch"
	"github.com/sidsharma96/concordance/internal/objectstore"
	"github.com/sidsharma96/concordance/internal/termsearch"
	"github.com/sidsharma96/concordance/internal/textterms"
)

func TestFreshClientReadSearchAndQuoteWithoutRegrouping(t *testing.T) {
	ctx, article, a, _, fields, locations := fixture(t)
	// This JSON is ONLY a synthetic transport fixture. It does not choose the
	// product's persisted index format. All three artifacts use real SDK storage.
	searchFixture, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	bodies := map[string][]byte{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		body, exists := bodies[r.URL.Path]
		switch r.Method {
		case "GET":
			if !exists {
				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(404)
				_, _ = io.WriteString(w, "<Error><Code>NoSuchKey</Code></Error>")
				return
			}
			_, _ = w.Write(body)
		case "PUT":
			if r.Header.Get("If-None-Match") != "*" {
				t.Error("missing conditional create")
			}
			if exists {
				w.WriteHeader(412)
				return
			}
			b, err := io.ReadAll(io.LimitReader(r.Body, (1<<20)+1))
			if err != nil || len(b) > 1<<20 {
				t.Error("fixture body", err)
				w.WriteHeader(400)
				return
			}
			bodies[r.URL.Path] = b
		default:
			t.Error("unexpected method")
			w.WriteHeader(405)
		}
	}))
	defer server.Close()
	fresh := func() *objectstore.Store {
		sdk := s3.New(s3.Options{Region: "auto", BaseEndpoint: aws.String(server.URL), UsePathStyle: true, Credentials: aws.AnonymousCredentials{}, HTTPClient: server.Client()})
		store, err := objectstore.New(sdk, "fixture", "authorized-collection", 1<<20)
		if err != nil {
			t.Fatal(err)
		}
		return store
	}
	refs := []objectstore.Ref{}
	for _, body := range [][]byte{article, locations, searchFixture} {
		ref, err := fresh().Ensure(ctx, body)
		if err != nil {
			t.Fatal(err)
		}
		refs = append(refs, ref)
	}
	recovered := [][]byte{}
	for _, ref := range refs {
		b, err := fresh().Read(ctx, ref)
		if err != nil {
			t.Fatal(err)
		}
		recovered = append(recovered, b)
	}
	var loaded Fields
	if err := json.Unmarshal(recovered[2], &loaded); err != nil {
		t.Fatal(err)
	}
	reader, err := Open(ctx, recovered[1], recovered[0], loaded, openLimits())
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range recovered {
		clear(b)
	}
	snapshot, err := htmlsearch.New(ctx, []htmlsearch.Edition{{ID: "one", Articles: []htmlarticle.Article{a}}}, htmlsearch.Limits{Fields: preparationLimits(), MaxEditions: 1, MaxArticles: 1, MaxBlocks: 1000, MaxGroups: 1000, MaxInputBytes: 1 << 20, MaxPassages: 1000, MaxTerms: 10000, MaxOutputBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	sl := termsearch.Limits{MaxEditions: 1, MaxPassages: 1000, MaxTerms: 10000, MaxQueryTerms: 10, MaxInputBytes: 1 << 20, MaxMatches: 1000}
	for _, text := range []string{"ContainerRestartRules default", "restartPolicy Never", "Pod"} {
		q, err := textterms.Prepare(ctx, text, preparationLimits().Field)
		if err != nil {
			t.Fatal(err)
		}
		query := termsearch.Query{TermsVersion: q.Version, Edition: "one", Terms: q.Terms}
		got, err := termsearch.RankBM25(ctx, termsearch.Corpus{TermsVersion: loaded.TermsVersion, Editions: []termsearch.Edition{{ID: "one", Passages: loaded.Passages}}}, query, sl)
		if err != nil {
			t.Fatal(err)
		}
		want, err := snapshot.Search(ctx, query, sl)
		if err != nil || len(got.Matches) != len(want.Hits) || len(got.Matches) == 0 {
			t.Fatal(got, err)
		}
		for i, hit := range got.Matches {
			expected, err := snapshot.Location(want.Hits[i].Ref)
			if err != nil {
				t.Fatal(err)
			}
			location, err := reader.Location(hit.Passage)
			if err != nil || location != expected.Body || hit.Score != want.Hits[i].Score {
				t.Fatal(location, err)
			}
			quote, err := reader.Quote(ctx, hit.Passage, 10000)
			if err != nil {
				t.Fatal(err)
			}
			original, err := snapshot.Quote(ctx, want.Hits[i].Ref, 10000)
			if err != nil || quote != original {
				t.Fatal(quote, err)
			}
		}
	}
}

func TestReaderObeysSavedRangesRatherThanCurrentGrouping(t *testing.T) {
	ctx := deadline(t)
	a := htmlarticle.Article{Processor: "htmlarticle/v1", Profile: "fixture/v1", HTMLBytes: 1, Blocks: []htmlarticle.Block{{Kind: htmlarticle.Prose, Text: "one"}, {Kind: htmlarticle.Prose, Text: "two"}, {Kind: htmlarticle.Prose, Text: "three"}}, Groups: []htmlarticle.Group{{Kind: htmlarticle.Notice, First: 0, After: 3, Label: "notice"}}}
	raw, err := htmlarticlejson.Encode(ctx, a, articleLimits())
	if err != nil {
		t.Fatal(err)
	}
	current, err := htmlpassage.Build(ctx, a, preparationLimits().Passages)
	if err != nil || len(current.Passages) != 1 {
		t.Fatal(current, err)
	}
	// Synthetic representation fixture, not a claimed historical production output:
	// the reader validates representation/bindings, not the claimed builder's work.
	f := Fields{passageVersion, fieldVersion, termsVersion, []termsearch.Fields{{Body: []string{"one"}}, {Body: []string{"two", "three"}}}}
	hash, err := fingerprint(ctx, f, limits())
	if err != nil {
		t.Fatal(err)
	}
	table := &Table{articleHash: sha256.Sum256(raw), articleBytes: len(raw), fieldsHash: hash, blocks: 3, rows: []htmlpassage.Range{{First: 0, After: 1}, {First: 1, After: 3}}}
	saved, err := Encode(ctx, table, limits())
	if err != nil {
		t.Fatal(err)
	}
	reader, err := Open(ctx, saved, raw, f, openLimits())
	if err != nil {
		t.Fatal(err)
	}
	if quote, err := reader.Quote(ctx, 1, 100); err != nil || quote != "two\nthree" {
		t.Fatal(quote, err)
	}
}

type interrupt struct {
	context.Context
	cancel      context.CancelFunc
	count, stop int
}

func (c *interrupt) Err() error {
	c.count++
	if c.stop > 0 && c.count >= c.stop {
		c.cancel()
	}
	return c.Context.Err()
}
func TestEveryObservedCooperativeCancellationBoundary(t *testing.T) {
	ctx, raw, _, table, f, saved := fixture(t)
	reader, err := Open(ctx, saved, raw, f, openLimits())
	if err != nil {
		t.Fatal(err)
	}
	invocations := []func(context.Context) error{
		func(c context.Context) error {
			table, f, err := Capture(c, raw, captureLimits())
			if err != nil && (table != nil || len(f.Passages) != 0) {
				t.Fatal("partial capture")
			}
			return err
		},
		func(c context.Context) error {
			b, err := Encode(c, table, limits())
			if err != nil && b != nil {
				t.Fatal("partial encode")
			}
			return err
		},
		func(c context.Context) error {
			r, err := Open(c, saved, raw, f, openLimits())
			if err != nil && r != nil {
				t.Fatal("partial open")
			}
			return err
		},
		func(c context.Context) error {
			q, err := reader.Quote(c, 0, 10000)
			if err != nil && q != "" {
				t.Fatal("partial quote")
			}
			return err
		},
	}
	for i, invoke := range invocations {
		parent, cancel := context.WithCancel(ctx)
		counter := &interrupt{Context: parent, cancel: cancel}
		if err := invoke(counter); err != nil {
			t.Fatal(err)
		}
		cancel()
		for stop := 1; stop <= counter.count; stop++ {
			parent, cancel := context.WithCancel(ctx)
			c := &interrupt{Context: parent, cancel: cancel, stop: stop}
			err := invoke(c)
			cancel()
			if !errors.Is(err, context.Canceled) {
				t.Fatal(i, stop, err)
			}
		}
	}
}
func FuzzDecodeRoundTrip(f *testing.F) {
	table := &Table{articleHash: [32]byte{1}, fieldsHash: [32]byte{2}, articleBytes: 3, blocks: 1, rows: []htmlpassage.Range{{First: 0, After: 1}}}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	saved, _ := Encode(ctx, table, limits())
	cancel()
	f.Add(saved)
	f.Add([]byte(`{"format":"concordance.html-locations/v1"}`))
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 8192 {
			t.Skip()
		}
		ctx := deadline(t)
		l := Limits{8192, 64, 64, 256, 256, 8192}
		got, err := decode(ctx, b, l)
		if err != nil {
			if got != nil {
				t.Fatal("partial decode")
			}
			return
		}
		encoded, err := Encode(ctx, got, l)
		if err != nil {
			t.Fatal(err)
		}
		again, err := decode(ctx, encoded, l)
		if err != nil {
			t.Fatal(err)
		}
		stable, err := Encode(ctx, again, l)
		if err != nil || !bytes.Equal(encoded, stable) {
			t.Fatal(err)
		}
	})
}

func TestFingerprintKnownVector(t *testing.T) {
	f := Fields{passageVersion, fieldVersion, termsVersion, []termsearch.Fields{{Body: []string{"pod", "restartPolicy"}, Headings: []string{"guide"}}}}
	got, err := fingerprint(deadline(t), f, limits())
	if err != nil {
		t.Fatal(err)
	}
	// Independent Python hashlib/struct reference, kept as a fixed byte vector.
	want, err := hex.DecodeString("a8e6fa8deab9d55ae55b54164d7fbfb4910eb4994977889f12fca21cebf76944")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got[:], want) {
		t.Fatal(hex.EncodeToString(got[:]))
	}
}
