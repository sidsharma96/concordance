package generationpublish

import (
	"bytes"
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/sidsharma96/concordance/internal/catalog"
	gm "github.com/sidsharma96/concordance/internal/generationmanifest"
	"github.com/sidsharma96/concordance/internal/htmlarticle"
	"github.com/sidsharma96/concordance/internal/htmlarticlejson"
	"github.com/sidsharma96/concordance/internal/htmledition"
	"github.com/sidsharma96/concordance/internal/htmlpassage"
	"github.com/sidsharma96/concordance/internal/htmlterms"
	"github.com/sidsharma96/concordance/internal/objectstore"
	"github.com/sidsharma96/concordance/internal/textterms"
)

func testContext(t testing.TB) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)
	return ctx
}
func testLimits() Limits {
	return Limits{MaxObjects: 64, MaxBatchObjects: 2, MaxInputBytes: 8 << 20, MaxBatchBytes: 1 << 20,
		Manifest: gm.Limits{MaxEncodedBytes: 1 << 20, MaxSources: 4, MaxEditions: 4, MaxCaptures: 8, MaxFiles: 32, MaxDocuments: 32, MaxImages: 32, MaxSourceFiles: 32, MaxSourceDocuments: 32, MaxStringBytes: 4096, MaxValueBytes: 1 << 18, MaxObjectBytes: 1 << 20, MaxReferencedBytes: 8 << 20},
		Edition:  htmledition.Limits{MaxEncodedBytes: 1 << 20, MaxArticles: 32, MaxArticleBytes: 1 << 20, MaxBlocks: 10000, MaxPassages: 1000, MaxTerms: 20000, MaxPreparedBytes: 1 << 20, MaxKeyBytes: 1 << 20, MaxTermBytes: 1024, MaxVocabulary: 10000, MaxPostings: 20000},
		Article:  htmlarticlejson.Limits{MaxEncodedBytes: 1 << 20, MaxBlocks: 1000, MaxGroups: 1000, MaxStringBytes: 65536, MaxValueBytes: 1 << 20, MaxGroupDepth: 64, MaxHTMLBytes: 1 << 20}}
}
func prepLimits() htmlterms.Limits {
	return htmlterms.Limits{Passages: htmlpassage.Limits{MaxBlocks: 1000, MaxGroups: 1000, MaxInputBytes: 1 << 20, MaxPassages: 1000, MaxBodyBytes: 4096, MaxReferences: 10000, MaxContextBytes: 1 << 20}, Field: textterms.Limits{MaxInputBytes: 65536, MaxTerms: 10000, MaxTermBytes: 1024, MaxOutputBytes: 1 << 20}, MaxViewBytes: 1 << 20, MaxTerms: 20000, MaxOutputBytes: 1 << 20}
}
func ref(b []byte) gm.Ref               { return gm.Ref{SHA256: sha256.Sum256(b), Bytes: int64(len(b))} }
func storeRef(r gm.Ref) objectstore.Ref { return objectstore.Ref{SHA256: r.SHA256, Bytes: r.Bytes} }

type bundle struct {
	d         gm.Description
	manifest  []byte
	artifacts [][]byte
}

// Controlled synthetic producer, not Git acquisition, a real website build or the
// complete captured-input fingerprint. Two logical documents share article bytes.
func makeBundle(t testing.TB) bundle {
	t.Helper()
	ctx := testContext(t)
	l := testLimits()
	var b bundle
	seen := map[gm.Ref]bool{}
	put := func(body []byte) gm.Ref {
		r := ref(body)
		if !seen[r] {
			seen[r] = true
			b.artifacts = append(b.artifacts, bytes.Clone(body))
		}
		return r
	}
	html := []byte(`<main class="fixture"><h1>Pods</h1><p>Retry uploads safely.</p><img src="/images/pod.svg" alt="Pod diagram"></main>`)
	a, err := htmlarticle.Extract(ctx, html, htmlarticle.Profile{ID: "fixture/v1", ArticleClass: "fixture"}, htmlarticle.Limits{MaxInputBytes: 1 << 20, MaxTokenBytes: 65536, MaxTokens: 10000, MaxNodes: 10000, MaxDepth: 64, MaxBlocks: 1000, MaxGroups: 1000, MaxBlockBytes: 65536, MaxOutputBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	article, err := htmlarticlejson.Encode(ctx, a, l.Article)
	if err != nil {
		t.Fatal(err)
	}
	index, err := htmledition.Build(ctx, "one", [][]byte{article, article}, htmledition.BuildLimits{File: l.Edition, Article: l.Article, Preparation: prepLimits()})
	if err != nil {
		t.Fatal(err)
	}
	wire, err := htmledition.Encode(ctx, index, l.Edition)
	if err != nil {
		t.Fatal(err)
	}
	empty, err := htmledition.Build(ctx, "empty", nil, htmledition.BuildLimits{File: l.Edition, Article: l.Article, Preparation: prepLimits()})
	if err != nil {
		t.Fatal(err)
	}
	emptyWire, err := htmledition.Encode(ctx, empty, l.Edition)
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte("# Pods\n\nRetry uploads safely.\n")
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><text>Pod</text></svg>`)
	blob := func(body []byte) string {
		return fmt.Sprintf("%x", sha1.Sum(append([]byte(fmt.Sprintf("blob %d\x00", len(body))), body...)))
	}
	c := gm.Capture{Source: "docs", Revision: strings.Repeat("a", 40), Directory: "docs", Preparation: "fixture/v1", Toolchain: "fixture/v1", Recipe: put([]byte("controlled synthetic producer")), Files: []gm.File{{Path: "docs/pods.md", BlobID: blob(raw), Object: put(raw)}, {Path: "docs/other.md", BlobID: blob(raw), Object: put(raw)}, {Path: "static/images/pod.svg", BlobID: blob(svg), Object: put(svg)}}}
	var images []gm.Image
	for i, block := range a.Blocks {
		if block.Kind == htmlarticle.Image {
			images = append(images, gm.Image{Block: i, Address: block.SourceURL, File: 2})
		}
	}
	doc := gm.Document{Capture: 0, File: 0, NativeID: c.Files[0].Path, Location: c.Files[0].Path, Profile: a.Profile, HTML: put(html), Article: put(article), Images: images}
	other := doc
	other.File = 1
	other.NativeID = c.Files[1].Path
	other.Location = c.Files[1].Path
	other.Images = append([]gm.Image{}, images...)
	b.d = gm.Description{Collection: "guide", Generation: "generation-1", Sources: []gm.Source{{ID: "docs", Repository: "https://example.invalid/docs.git"}}, Editions: []gm.Edition{{ID: "one", Index: put(wire), Captures: []gm.Capture{c}, Documents: []gm.Document{doc, other}}, {ID: "empty", Index: put(emptyWire)}}}
	b.encode(t)
	return b
}
func (b *bundle) encode(t testing.TB) {
	t.Helper()
	var err error
	b.manifest, err = gm.Encode(testContext(t), b.d, testLimits().Manifest)
	if err != nil {
		t.Fatal(err)
	}
}
func trustedRequest(key string) catalog.Request {
	// This low-level fixture supplies trusted synthetic identity. It is not the
	// production fingerprint algorithm or permission to expose caller digests.
	return catalog.Request{Collection: "guide", Key: key, FingerprintVersion: "controlled-fixture/v1", Fingerprint: sha256.Sum256([]byte("synthetic captured inputs and settings"))}
}
func referenceClient(t testing.TB) *objectstore.Store {
	t.Helper()
	c := s3.New(s3.Options{Region: "auto", BaseEndpoint: aws.String("https://example.invalid"), UsePathStyle: true, Credentials: aws.AnonymousCredentials{}})
	o, err := objectstore.New(c, "fixture", "guide", 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	return o
}
