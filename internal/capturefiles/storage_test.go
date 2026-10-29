package capturefiles

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sidsharma96/concordance/internal/catalog"
	gm "github.com/sidsharma96/concordance/internal/generationmanifest"
	"github.com/sidsharma96/concordance/internal/generationpublish"
	"github.com/sidsharma96/concordance/internal/gitsource"
	"github.com/sidsharma96/concordance/internal/htmlarticle"
	"github.com/sidsharma96/concordance/internal/htmlarticlejson"
	"github.com/sidsharma96/concordance/internal/htmledition"
	"github.com/sidsharma96/concordance/internal/htmlpassage"
	"github.com/sidsharma96/concordance/internal/htmlterms"
	"github.com/sidsharma96/concordance/internal/objectstore"
	"github.com/sidsharma96/concordance/internal/preparedimport"
	"github.com/sidsharma96/concordance/internal/termsearch"
	"github.com/sidsharma96/concordance/internal/textterms"
)

func importLimits() preparedimport.Limits {
	a := htmlarticlejson.Limits{MaxEncodedBytes: 1 << 20, MaxBlocks: 1000, MaxGroups: 1000, MaxStringBytes: 65536, MaxValueBytes: 1 << 20, MaxGroupDepth: 64, MaxHTMLBytes: 1 << 20}
	e := htmledition.Limits{MaxEncodedBytes: 1 << 20, MaxArticles: 32, MaxArticleBytes: 1 << 20, MaxBlocks: 10000, MaxPassages: 1000, MaxTerms: 20000, MaxPreparedBytes: 1 << 20, MaxKeyBytes: 1 << 20, MaxTermBytes: 1024, MaxVocabulary: 10000, MaxPostings: 20000}
	prep := htmlterms.Limits{Passages: htmlpassage.Limits{MaxBlocks: 1000, MaxGroups: 1000, MaxInputBytes: 1 << 20, MaxPassages: 1000, MaxBodyBytes: 4096, MaxReferences: 10000, MaxContextBytes: 1 << 20}, Field: textterms.Limits{MaxInputBytes: 65536, MaxTerms: 10000, MaxTermBytes: 1024, MaxOutputBytes: 1 << 20}, MaxViewBytes: 1 << 20, MaxTerms: 20000, MaxOutputBytes: 1 << 20}
	return preparedimport.Limits{MaxInputBytes: 8 << 20, MaxStagedBytes: 8 << 20, MaxObjects: 64, MaxHashBytes: 1 << 20, Manifest: gm.Limits{MaxEncodedBytes: 1 << 20, MaxSources: 4, MaxEditions: 4, MaxCaptures: 8, MaxFiles: 64, MaxDocuments: 32, MaxImages: 32, MaxSourceFiles: 32, MaxSourceDocuments: 32, MaxStringBytes: 4096, MaxValueBytes: 1 << 18, MaxObjectBytes: 1 << 20, MaxReferencedBytes: 8 << 20}, Git: gitsource.Limits{MaxEntries: 64, MaxListingBytes: 1 << 16, MaxDocumentBytes: 1 << 20, MaxTotalBytes: 2 << 20}, Files: gitsource.FileLimits{MaxFiles: 32, MaxListingBytes: 1 << 16, MaxFileBytes: 1 << 20, MaxTotalBytes: 2 << 20}, Extract: htmlarticle.Limits{MaxInputBytes: 1 << 20, MaxTokenBytes: 65536, MaxTokens: 10000, MaxNodes: 10000, MaxDepth: 64, MaxBlocks: 1000, MaxGroups: 1000, MaxBlockBytes: 65536, MaxOutputBytes: 1 << 20}, Edition: htmledition.BuildLimits{File: e, Article: a, Preparation: prep}}
}

type testObjects struct {
	mu     sync.Mutex
	bodies map[string][]byte
	calls  int
}

func (s *testObjects) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	b, ok := s.bodies[r.URL.Path]
	switch r.Method {
	case "PUT":
		if r.Header.Get("If-None-Match") != "*" {
			w.WriteHeader(400)
			return
		}
		if ok {
			w.WriteHeader(412)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, (1<<20)+1))
		if err != nil || len(body) > 1<<20 {
			w.WriteHeader(400)
			return
		}
		s.bodies[r.URL.Path] = body
	case "GET":
		if !ok {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(404)
			_, _ = io.WriteString(w, "<Error><Code>NoSuchKey</Code></Error>")
			return
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(b)))
		w.Header().Set("ETag", fmt.Sprintf(`"opaque-%x"`, sha256.Sum256(b)))
		_, _ = w.Write(b)
	default:
		w.WriteHeader(405)
	}
}
func (s *testObjects) count() int { s.mu.Lock(); defer s.mu.Unlock(); return s.calls }

func TestCaptureDirectoryFeedsPublishedSearch(t *testing.T) {
	dsn := os.Getenv("CONCORDANCE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set CONCORDANCE_TEST_DATABASE_URL to a dedicated test database")
	}
	ctx := testContext(t)
	repo := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.CommandContext(ctx, "git", append([]string{"-C", repo, "-c", "core.hooksPath="}, args...)...)
		for _, e := range os.Environ() {
			if !strings.HasPrefix(e, "GIT_") {
				cmd.Env = append(cmd.Env, e)
			}
		}
		cmd.Env = append(cmd.Env, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.invalid", "GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.invalid")
		b, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("fixture git: %v: %s", err, b)
		}
		return strings.TrimSpace(string(b))
	}
	git("init", "--object-format=sha1")
	for _, path := range []string{"docs/a.md", "docs/b.md"} {
		writeFile(t, repo, path, []byte("# Pods\n\nRetry uploads safely.\n"))
	}
	writeFile(t, repo, "includes/sample.yaml", []byte("kind: Pod\n"))
	git("add", "--all")
	git("commit", "-m", "synthetic capture files")
	pin := git("rev-parse", "HEAD")
	d := description()
	d.Editions[0].Captures[0].Revision = pin
	root := writeCapture(t, d)
	editions, err := ReadDirectory(ctx, root, testLimits())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(root); err != nil {
		t.Fatal(err)
	} // Import must use the verified buffers.
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	schema := pgx.Identifier{"capture_" + strings.ToLower(rand.Text())}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := admin.Exec(c, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Error(err)
		}
	})
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	cat, err := catalog.New(pool)
	if err != nil {
		t.Fatal(err)
	}
	if err := cat.CreateSchema(ctx); err != nil {
		t.Fatal(err)
	}
	if err := cat.CreateReceiptSchema(ctx); err != nil {
		t.Fatal(err)
	}
	remote := &testObjects{bodies: map[string][]byte{}}
	server := httptest.NewServer(remote)
	t.Cleanup(server.Close)
	objects := func() *objectstore.Store {
		t.Helper()
		sdk := s3.New(s3.Options{Region: "auto", BaseEndpoint: aws.String(server.URL), UsePathStyle: true, Credentials: aws.AnonymousCredentials{}, HTTPClient: server.Client()})
		o, e := objectstore.New(sdk, "fixture", "guide", 1<<20)
		if e != nil {
			t.Fatal(e)
		}
		return o
	}
	o := objects()
	ns, err := cat.BindNamespace(ctx, "fixture", o.Binding())
	if err != nil {
		t.Fatal(err)
	}
	l := importLimits()
	p, err := generationpublish.New(cat, o, ns, generationpublish.Limits{MaxObjects: l.MaxObjects, MaxBatchObjects: 2, MaxInputBytes: l.MaxStagedBytes, MaxBatchBytes: 1 << 20, Manifest: l.Manifest, Edition: l.Edition.File, Article: l.Edition.Article})
	if err != nil {
		t.Fatal(err)
	}
	importer, err := preparedimport.New(p, "guide", []preparedimport.Source{{ID: "docs", Repository: "https://example.invalid/docs.git", LocalPath: repo}}, l)
	if err != nil {
		t.Fatal(err)
	}
	first, err := importer.ImportPrepared(ctx, "saved", "candidate", editions)
	if err != nil {
		t.Fatal(err)
	}
	before := remote.count()
	retry, err := importer.ImportPrepared(ctx, "saved", "new-candidate", editions)
	if err != nil || !retry.Duplicate || retry.Generation != first.Generation || remote.count() != before {
		t.Fatal(retry, err)
	}
	for _, ed := range editions {
		for _, c := range ed.Captures {
			clear(c.Recipe)
			for _, page := range c.Pages {
				clear(page.HTML)
			}
		}
	}
	if err := os.RemoveAll(repo); err != nil {
		t.Fatal(err)
	}
	fresh := objects()
	body, err := fresh.Read(ctx, objectstore.Ref{SHA256: first.Generation.ManifestSHA256, Bytes: first.Generation.ManifestBytes})
	if err != nil {
		t.Fatal(err)
	}
	m, err := gm.Open(ctx, body, gm.Ref{SHA256: first.Generation.ManifestSHA256, Bytes: first.Generation.ManifestBytes}, "guide", "candidate", l.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	desc, err := m.Describe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ref := desc.Editions[0].Index
	body, err = fresh.Read(ctx, objectstore.Ref{SHA256: ref.SHA256, Bytes: ref.Bytes})
	if err != nil {
		t.Fatal(err)
	}
	index, err := m.OpenEdition(ctx, "one", body, l.Edition.File)
	if err != nil {
		t.Fatal(err)
	}
	hits, err := index.Search(ctx, termsearch.Query{TermsVersion: textterms.Version, Edition: "one", Terms: []string{"retry"}}, termsearch.Limits{MaxEditions: 1, MaxPassages: 1000, MaxTerms: 20000, MaxQueryTerms: 16, MaxInputBytes: 1 << 20, MaxMatches: 1000})
	if err != nil || len(hits.Matches) != 2 {
		t.Fatal(hits, err)
	}
	loc, err := index.Location(hits.Matches[0].Passage)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := m.Resolve(ctx, "one", loc.Article)
	if err != nil || binding.NativeID != "docs/a.md" || binding.HTML.SHA256 != sha256.Sum256([]byte(htmlBody)) {
		t.Fatal(binding, err)
	}
	body, err = fresh.Read(ctx, objectstore.Ref{SHA256: binding.Article.SHA256, Bytes: binding.Article.Bytes})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.VerifyArticle(ctx, "one", loc.Article, body, l.Edition.Article); err != nil {
		t.Fatal(err)
	}
	quote, err := index.Quote(ctx, hits.Matches[0].Passage, body, l.Edition.Article, 4096)
	if err != nil || quote != "Pods\nRetry uploads safely." {
		t.Fatal(quote, err)
	}
	t.Logf("capture removed before import; source removed after publication; verified quote=%q", quote)
}
