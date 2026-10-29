package generationmanifest

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/sidsharma96/concordance/internal/objectstore"
	"github.com/sidsharma96/concordance/internal/termsearch"
	"github.com/sidsharma96/concordance/internal/textterms"
)

func TestFreshClientManifestIndexAndQuote(t *testing.T) {
	ctx := testContext(t)
	f := fixture(t)
	var mu sync.Mutex
	bodies := map[string][]byte{}
	gets := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		body, exists := bodies[r.URL.Path]
		switch r.Method {
		case "GET":
			gets++
			if !exists {
				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(404)
				_, _ = io.WriteString(w, "<Error><Code>NoSuchKey</Code></Error>")
				return
			}
			_, _ = w.Write(body)
		case "PUT":
			if r.Header.Get("If-None-Match") != "*" {
				t.Error("unconditional put")
				w.WriteHeader(400)
				return
			}
			if exists {
				w.WriteHeader(412)
				return
			}
			b, err := io.ReadAll(io.LimitReader(r.Body, (1<<20)+1))
			if err != nil || len(b) > 1<<20 {
				t.Error("invalid fixture body", err)
				w.WriteHeader(400)
				return
			}
			bodies[r.URL.Path] = b
		default:
			w.WriteHeader(405)
		}
	}))
	defer server.Close()
	fresh := func() *objectstore.Store {
		sdk := s3.New(s3.Options{Region: "auto", BaseEndpoint: aws.String(server.URL), UsePathStyle: true, Credentials: aws.AnonymousCredentials{}, HTTPClient: server.Client()})
		s, err := objectstore.New(sdk, "fixture", f.d.Collection, 1<<20)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	toStore := func(r Ref) objectstore.Ref { return objectstore.Ref{SHA256: r.SHA256, Bytes: r.Bytes} }
	// All declared artifacts are confirmed first. This fixture retains the eventual
	// manifest reference in memory, not PostgreSQL publication or host-loss recovery.
	wire := encoded(t, f.d)
	m := opened(t, wire)
	refs, err := m.Objects(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range refs {
		got, err := fresh().Ensure(ctx, f.objects[r])
		if err != nil || got != toStore(r) {
			t.Fatal("artifact confirmation", err)
		}
	}
	published, err := fresh().Ensure(ctx, wire)
	if err != nil {
		t.Fatal(err)
	}
	clear(wire)
	clear(f.article)
	clear(f.index)
	for _, b := range f.objects {
		clear(b)
	}
	mu.Lock()
	gets = 0
	mu.Unlock()
	manifestBody, err := fresh().Read(ctx, published)
	if err != nil {
		t.Fatal(err)
	}
	m, err = Open(ctx, manifestBody, Ref{published.SHA256, published.Bytes}, f.d.Collection, f.d.Generation, testLimits())
	if err != nil {
		t.Fatal(err)
	}
	clear(manifestBody)
	d, err := m.Describe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	indexBody, err := fresh().Read(ctx, toStore(d.Editions[0].Index))
	if err != nil {
		t.Fatal(err)
	}
	x, err := m.OpenEdition(ctx, "one", indexBody, indexLimits())
	if err != nil {
		t.Fatal(err)
	}
	clear(indexBody)
	mu.Lock()
	count := gets
	mu.Unlock()
	if count != 2 {
		t.Fatalf("opening manifest+index fetched %d bodies", count)
	}
	q, err := textterms.Prepare(ctx, "retry", textterms.Limits{MaxInputBytes: 256, MaxTerms: 16, MaxTermBytes: 256, MaxOutputBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	hits, err := x.Search(ctx, termsearch.Query{TermsVersion: q.Version, Edition: "one", Terms: q.Terms}, termsearch.Limits{MaxEditions: 1, MaxPassages: 1000, MaxTerms: 20000, MaxQueryTerms: 16, MaxInputBytes: 1 << 20, MaxMatches: 1000})
	if err != nil || len(hits.Matches) != 2 {
		t.Fatal("search", err)
	}
	loc, err := x.Location(hits.Matches[0].Passage)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := m.Resolve(ctx, "one", loc.Article)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Source.ID != "docs" || doc.NativeID != doc.Input.Path {
		t.Fatal("lost source association")
	}
	articleBody, err := fresh().Read(ctx, toStore(doc.Article))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.VerifyArticle(ctx, "one", loc.Article, articleBody, articleLimits()); err != nil {
		t.Fatal(err)
	}
	quote, err := x.Quote(ctx, hits.Matches[0].Passage, articleBody, articleLimits(), 4096)
	if err != nil || !bytes.Contains([]byte(quote), []byte("Retry uploads safely.")) {
		t.Fatal("quote", err)
	}
	clear(articleBody)
	mu.Lock()
	count = gets
	mu.Unlock()
	if count != 3 {
		t.Fatalf("quote reads: %d", count)
	}
	key, err := fresh().Key(toStore(doc.Article))
	if err != nil {
		t.Fatal(err)
	}
	articlePath := "/fixture/" + key
	mu.Lock()
	corrupt := bytes.Clone(bodies[articlePath])
	corrupt[0] ^= 1
	bodies[articlePath] = corrupt
	mu.Unlock()
	if b, err := fresh().Read(ctx, toStore(doc.Article)); !errors.Is(err, objectstore.ErrIntegrity) || b != nil {
		t.Fatalf("corrupt declared article: %v", err)
	}
	mu.Lock()
	delete(bodies, articlePath)
	mu.Unlock()
	if b, err := fresh().Read(ctx, toStore(doc.Article)); !errors.Is(err, objectstore.ErrNotFound) || b != nil {
		t.Fatalf("missing declared article: %v", err)
	}
	key, err = fresh().Key(toStore(d.Editions[0].Index))
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	delete(bodies, "/fixture/"+key)
	mu.Unlock()
	if b, err := fresh().Read(ctx, toStore(d.Editions[0].Index)); !errors.Is(err, objectstore.ErrNotFound) || b != nil {
		t.Fatalf("missing declared index: %v", err)
	}
}
