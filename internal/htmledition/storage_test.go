package htmledition

import (
	"bytes"
	"encoding/hex"
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
)

func TestFreshObjectClientLoadsEditionThenVerifiedQuote(t *testing.T) {
	ctx, articles, _, _, saved := fixture(t)
	var mu sync.Mutex
	bodies := map[string][]byte{}
	gets := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		body, exists := bodies[r.URL.Path]
		switch r.Method {
		case "GET":
			gets[r.URL.Path]++
			if !exists {
				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(404)
				_, _ = io.WriteString(w, "<Error><Code>NoSuchKey</Code></Error>")
				return
			}
			_, _ = w.Write(body)
		case "PUT":
			if r.Header.Get("If-None-Match") != "*" {
				t.Error("unconditional write")
			}
			if exists {
				w.WriteHeader(412)
				return
			}
			b, err := io.ReadAll(io.LimitReader(r.Body, (1<<20)+1))
			if err != nil || len(b) > 1<<20 {
				t.Error(err)
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
		s, err := objectstore.New(sdk, "fixture", "authorized-collection", 1<<20)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	refs := []objectstore.Ref{}
	for _, body := range append([][]byte{saved}, articles...) {
		ref, err := fresh().Ensure(ctx, body)
		if err != nil {
			t.Fatal(err)
		}
		refs = append(refs, ref)
	}
	for _, a := range articles {
		clear(a)
	}
	clear(saved)
	mu.Lock()
	clear(gets)
	mu.Unlock()
	raw, err := fresh().Read(ctx, refs[0])
	if err != nil {
		t.Fatal(err)
	}
	e, err := Open(ctx, raw, Ref{refs[0].SHA256, int(refs[0].Bytes)}, "one", limits())
	if err != nil {
		t.Fatal(err)
	}
	clear(raw)
	q := termsearch.Query{TermsVersion: termsVersion, Edition: "one", Terms: []string{"containerrestartrules", "default"}}
	result, err := e.Search(ctx, q, searchLimits())
	if err != nil || len(result.Matches) != 1 {
		t.Fatal(result, err)
	}
	mu.Lock()
	totalGets := 0
	for _, n := range gets {
		totalGets += n
	}
	mu.Unlock()
	if totalGets != 1 {
		t.Fatal("index open fetched article bodies", totalGets)
	}
	loc, err := e.Location(result.Matches[0].Passage)
	if err != nil {
		t.Fatal(err)
	}
	articleRef := refs[loc.Article+1]
	descriptor, err := e.Article(loc.Article)
	if err != nil || descriptor.Object.SHA256 != articleRef.SHA256 {
		t.Fatal(err)
	}
	articleBody, err := fresh().Read(ctx, articleRef)
	if err != nil {
		t.Fatal(err)
	}
	quote, err := e.Quote(ctx, result.Matches[0].Passage, articleBody, articleLimits(), 10000)
	if err != nil || !bytes.Contains([]byte(quote), []byte("Enabled by default.")) {
		t.Fatal(quote, err)
	}
	clear(articleBody)
	if !bytes.Contains([]byte(quote), []byte("Enabled by default.")) {
		t.Fatal("quote borrowed bytes")
	}
	key, err := fresh().Key(articleRef)
	if err != nil {
		t.Fatal(err)
	}
	path := "/fixture/" + key
	mu.Lock()
	corrupt := bytes.Clone(bodies[path])
	if len(corrupt) == 0 {
		mu.Unlock()
		t.Fatal("fixture key not found", hex.EncodeToString(articleRef.SHA256[:]))
	}
	corrupt[len(corrupt)-1] ^= 1
	bodies[path] = corrupt
	mu.Unlock()
	if body, err := fresh().Read(ctx, articleRef); body != nil || !errors.Is(err, objectstore.ErrIntegrity) {
		t.Fatal("accepted corrupt article", err)
	}
	mu.Lock()
	delete(bodies, path)
	mu.Unlock()
	if body, err := fresh().Read(ctx, articleRef); body != nil || !errors.Is(err, objectstore.ErrNotFound) {
		t.Fatal("missing article fallback", err)
	}
}
