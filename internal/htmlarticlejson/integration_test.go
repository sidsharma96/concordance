package htmlarticlejson

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/sidsharma96/concordance/internal/htmlarticle"
	"github.com/sidsharma96/concordance/internal/htmlpassage"
	"github.com/sidsharma96/concordance/internal/htmlsearch"
	"github.com/sidsharma96/concordance/internal/htmlterms"
	"github.com/sidsharma96/concordance/internal/objectstore"
	"github.com/sidsharma96/concordance/internal/termsearch"
	"github.com/sidsharma96/concordance/internal/textterms"
)

// Exercise real SDK byte storage and fresh-client reconstruction. This is an HTTP
// fixture, not provider durability, an importer or complete manifest publication.
func TestObjectRecoveryPreservesSearchAndQuotes(t *testing.T) {
	ctx := deadline(t)
	source := []byte("<article class=\"doc\"><h1>Pod</h1><div class=\"notice\" title=\"ContainerRestartRules\"><p>Enabled by default.</p></div><pre>  restartPolicy: Never\n</pre></article>")
	a, e := htmlarticle.Extract(ctx, source, htmlarticle.Profile{ID: "fixture/v1", ArticleClass: "doc", NoticeClasses: []string{"notice"}}, htmlarticle.Limits{MaxInputBytes: 10000, MaxTokenBytes: 10000, MaxTokens: 1000, MaxNodes: 1000, MaxDepth: 100, MaxBlocks: 100, MaxGroups: 100, MaxBlockBytes: 10000, MaxOutputBytes: 100000})
	if e != nil {
		t.Fatal(e)
	}
	encoded, e := Encode(ctx, a, limits())
	if e != nil {
		t.Fatal(e)
	}
	var mu sync.Mutex
	var stored []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch r.Method {
		case "GET":
			if stored == nil {
				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(404)
				_, _ = io.WriteString(w, "<Error><Code>NoSuchKey</Code></Error>")
				return
			}
			_, _ = w.Write(stored)
		case "PUT":
			if r.Header.Get("If-None-Match") != "*" {
				t.Error("missing conditional create")
			}
			if stored != nil {
				w.WriteHeader(412)
				return
			}
			var e error
			stored, e = io.ReadAll(io.LimitReader(r.Body, 1<<20))
			if e != nil {
				t.Error(e)
			}
		default:
			t.Error("unexpected method")
			w.WriteHeader(405)
		}
	}))
	defer server.Close()
	fresh := func() *objectstore.Store {
		client := s3.New(s3.Options{Region: "auto", BaseEndpoint: aws.String(server.URL), UsePathStyle: true, Credentials: aws.AnonymousCredentials{}, HTTPClient: server.Client()})
		s, e := objectstore.New(client, "fixture", "authorized-collection", 1<<20)
		if e != nil {
			t.Fatal(e)
		}
		return s
	}
	ref, e := fresh().Ensure(ctx, encoded)
	if e != nil {
		t.Fatal(e)
	}
	recovered, e := fresh().Read(ctx, ref)
	if e != nil {
		t.Fatal(e)
	}
	decoded, e := Decode(ctx, recovered, limits())
	if e != nil || !equal(a, decoded) {
		t.Fatal(decoded, e)
	}
	clear(recovered)
	again, e := Encode(ctx, decoded, limits())
	if e != nil || !bytes.Equal(again, encoded) {
		t.Fatal("decoded data borrowed storage bytes", e)
	}
	fl := textterms.Limits{MaxInputBytes: 10000, MaxTerms: 1000, MaxTermBytes: 10000, MaxOutputBytes: 100000}
	sl := htmlsearch.Limits{Fields: htmlterms.Limits{Passages: htmlpassage.Limits{MaxBlocks: 100, MaxGroups: 100, MaxInputBytes: 100000, MaxPassages: 100, MaxBodyBytes: 10000, MaxReferences: 1000, MaxContextBytes: 100000}, Field: fl, MaxViewBytes: 100000, MaxTerms: 10000, MaxOutputBytes: 100000}, MaxEditions: 1, MaxArticles: 1, MaxBlocks: 100, MaxGroups: 100, MaxInputBytes: 100000, MaxPassages: 100, MaxTerms: 10000, MaxOutputBytes: 100000}
	snapshot, e := htmlsearch.New(ctx, []htmlsearch.Edition{{ID: "one", Articles: []htmlarticle.Article{decoded}}}, sl)
	if e != nil {
		t.Fatal(e)
	}
	for _, query := range []string{"ContainerRestartRules default", "restartPolicy Never"} {
		prepared, e := textterms.Prepare(ctx, query, fl)
		if e != nil {
			t.Fatal(e)
		}
		result, e := snapshot.Search(ctx, termsearch.Query{TermsVersion: prepared.Version, Edition: "one", Terms: prepared.Terms}, termsearch.Limits{MaxEditions: 1, MaxPassages: 100, MaxTerms: 10000, MaxQueryTerms: 10, MaxInputBytes: 100000, MaxMatches: 100})
		if e != nil || len(result.Hits) != 1 {
			t.Fatal(result, e)
		}
		hit := result.Hits[0]
		location, e := snapshot.Location(hit.Ref)
		if e != nil {
			t.Fatal(e)
		}
		var texts []string
		for _, b := range a.Blocks[location.Body.First:location.Body.After] {
			texts = append(texts, b.Text)
		}
		quote, e := snapshot.Quote(ctx, hit.Ref, 10000)
		if e != nil || quote != strings.Join(texts, "\n") || strings.Contains(quote, "ContainerRestartRules") {
			t.Fatal(quote, e)
		}
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
func TestEveryCooperativeCancellationBoundary(t *testing.T) {
	ctx := deadline(t)
	a := sample()
	raw, _ := Encode(ctx, a, limits())
	for _, decode := range []bool{false, true} {
		invoke := func(c context.Context) error {
			if decode {
				a, e := Decode(c, raw, limits())
				if e != nil && len(a.Blocks) != 0 {
					t.Fatal("partial decoded article")
				}
				return e
			}
			b, e := Encode(c, a, limits())
			if e != nil && b != nil {
				t.Fatal("partial encoded bytes")
			}
			return e
		}
		parent, cancel := context.WithCancel(ctx)
		counter := &interrupt{Context: parent, cancel: cancel}
		if e := invoke(counter); e != nil {
			t.Fatal(e)
		}
		cancel()
		for stop := 1; stop <= counter.count; stop++ {
			parent, cancel := context.WithCancel(ctx)
			c := &interrupt{Context: parent, cancel: cancel, stop: stop}
			e := invoke(c)
			cancel()
			if e != context.Canceled {
				t.Fatal(decode, stop, e)
			}
		}
	}
}
