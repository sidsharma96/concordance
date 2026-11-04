package importjob

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
	"path/filepath"
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
	"github.com/sidsharma96/concordance/internal/publishedsearch"
	"github.com/sidsharma96/concordance/internal/termsearch"
	"github.com/sidsharma96/concordance/internal/textterms"
)

func importLimits() preparedimport.Limits {
	article := htmlarticlejson.Limits{MaxEncodedBytes: 1 << 20, MaxBlocks: 1000, MaxGroups: 1000, MaxStringBytes: 65536, MaxValueBytes: 1 << 20, MaxGroupDepth: 64, MaxHTMLBytes: 1 << 20}
	edition := htmledition.Limits{MaxEncodedBytes: 1 << 20, MaxArticles: 32, MaxArticleBytes: 1 << 20, MaxBlocks: 10000, MaxPassages: 1000, MaxTerms: 20000, MaxPreparedBytes: 1 << 20, MaxKeyBytes: 1 << 20, MaxTermBytes: 1024, MaxVocabulary: 10000, MaxPostings: 20000}
	passage := htmlpassage.Limits{MaxBlocks: 1000, MaxGroups: 1000, MaxInputBytes: 1 << 20, MaxPassages: 1000, MaxBodyBytes: 4096, MaxReferences: 10000, MaxContextBytes: 1 << 20}
	prep := htmlterms.Limits{Passages: passage, Field: textterms.Limits{MaxInputBytes: 65536, MaxTerms: 10000, MaxTermBytes: 1024, MaxOutputBytes: 1 << 20}, MaxViewBytes: 1 << 20, MaxTerms: 20000, MaxOutputBytes: 1 << 20}
	return preparedimport.Limits{
		MaxInputBytes: 8 << 20, MaxStagedBytes: 8 << 20, MaxObjects: 64, MaxHashBytes: 1 << 20,
		Manifest: gm.Limits{MaxEncodedBytes: 1 << 20, MaxSources: 4, MaxEditions: 4, MaxCaptures: 8, MaxFiles: 64, MaxDocuments: 32, MaxImages: 32, MaxSourceFiles: 32, MaxSourceDocuments: 32, MaxStringBytes: 4096, MaxValueBytes: 1 << 18, MaxObjectBytes: 1 << 20, MaxReferencedBytes: 8 << 20},
		Git:      gitsource.Limits{MaxEntries: 64, MaxListingBytes: 1 << 16, MaxDocumentBytes: 1 << 20, MaxTotalBytes: 2 << 20},
		Files:    gitsource.FileLimits{MaxFiles: 32, MaxListingBytes: 1 << 16, MaxFileBytes: 1 << 20, MaxTotalBytes: 2 << 20},
		Extract:  htmlarticle.Limits{MaxInputBytes: 1 << 20, MaxTokenBytes: 65536, MaxTokens: 10000, MaxNodes: 10000, MaxDepth: 64, MaxBlocks: 1000, MaxGroups: 1000, MaxBlockBytes: 65536, MaxOutputBytes: 1 << 20},
		Edition:  htmledition.BuildLimits{File: edition, Article: article, Preparation: prep},
	}
}

type localObjects struct {
	mu     sync.Mutex
	bodies map[string][]byte
	calls  int
}

func (s *localObjects) count() int { s.mu.Lock(); defer s.mu.Unlock(); return s.calls }
func (s *localObjects) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	body, found := s.bodies[r.URL.Path]
	if r.Method == "PUT" {
		if r.Header.Get("If-None-Match") != "*" {
			w.WriteHeader(400)
			return
		}
		if found {
			w.WriteHeader(412)
			return
		}
		data, err := io.ReadAll(io.LimitReader(r.Body, (1<<20)+1))
		if err != nil || len(data) > 1<<20 {
			w.WriteHeader(400)
			return
		}
		s.bodies[r.URL.Path] = data
		return
	}
	if r.Method != "GET" && r.Method != "HEAD" {
		w.WriteHeader(405)
		return
	}
	if !found {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(404)
		_, _ = io.WriteString(w, "<Error><Code>NoSuchKey</Code></Error>")
		return
	}
	etag := fmt.Sprintf(`"body-%x"`, sha256.Sum256(body))
	if match := r.Header.Get("If-Match"); match != "" && match != etag {
		w.WriteHeader(412)
		return
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.Header().Set("ETag", etag)
	if r.Method == "GET" {
		_, _ = w.Write(body)
	}
}
func TestOwnedJobFeedsPublishedImportAndCompletedRetry(t *testing.T) {
	dsn := os.Getenv("CONCORDANCE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set a dedicated local test PostgreSQL database")
	}
	ctx := testContext(t)
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, "docs"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.md", "b.md"} {
		if err := os.WriteFile(filepath.Join(repo, "docs", name), []byte("# Pods\nRetry uploads safely.\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	git := func(args ...string) string {
		cmd := exec.CommandContext(ctx, "git", append([]string{"-C", repo, "-c", "core.hooksPath=", "-c", "commit.gpgSign=false"}, args...)...)
		for _, entry := range os.Environ() {
			if !strings.HasPrefix(entry, "GIT_") {
				cmd.Env = append(cmd.Env, entry)
			}
		}
		body, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("synthetic git command failed: %v", err)
		}
		return strings.TrimSpace(string(body))
	}
	git("init", "--quiet")
	git("config", "user.name", "Fixture")
	git("config", "user.email", "fixture@example.invalid")
	git("add", ".")
	git("commit", "--quiet", "-m", "fixture")
	producer := writeCapture(t, git("rev-parse", "HEAD"))
	directory := filepath.Join(t.TempDir(), "job")
	saved, err := Prepare(ctx, producer, directory, fixtureRequest(), testLimits())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(producer); err != nil {
		t.Fatal(err)
	}
	var token [8]byte
	if _, err := rand.Read(token[:]); err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("job_fixture_%x", token)
	quoted := pgx.Identifier{schema}.Sanitize()
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal("connect local test database", err)
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := admin.Close(cleanup); err != nil {
			t.Error(err)
		}
	}()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := admin.Exec(cleanup, "DROP SCHEMA "+quoted+" CASCADE"); err != nil {
			t.Error(err)
		}
	}()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = quoted
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
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
	peer := &localObjects{bodies: map[string][]byte{}}
	server := httptest.NewServer(peer)
	defer server.Close()
	sdk := s3.New(s3.Options{Region: "auto", BaseEndpoint: aws.String(server.URL), UsePathStyle: true, Credentials: aws.AnonymousCredentials{}, HTTPClient: server.Client()})
	objects, err := objectstore.New(sdk, "fixture", "guide", 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	namespace, err := cat.BindNamespace(ctx, "fixture", objects.Binding())
	if err != nil {
		t.Fatal(err)
	}
	publisher, err := generationpublish.New(cat, objects, namespace, generationpublish.Limits{Manifest: importLimits().Manifest, Edition: importLimits().Edition.File, Article: importLimits().Edition.Article, MaxObjects: 64, MaxInputBytes: 8 << 20, MaxBatchObjects: 32, MaxBatchBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	importer, err := preparedimport.New(publisher, "guide", []preparedimport.Source{{ID: "docs", Repository: fixtureRequest().Sources[0].Repository, LocalPath: repo}}, importLimits())
	if err != nil {
		t.Fatal(err)
	}
	job, err := Open(ctx, directory, testLimits())
	if err != nil {
		t.Fatal(err)
	}
	result, err := importer.ImportPrepared(ctx, job.Key, job.CandidateID, job.Editions)
	if err != nil {
		t.Fatal(err)
	}
	if result.Generation.ID != saved.CandidateID || result.Duplicate {
		t.Fatal(result)
	}
	// Quarantine blocks receipt-based writes, while completed-key recovery is still
	// allowed with this already captured namespace handle and local preflight.
	if err := cat.Quarantine(ctx, namespace); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(ctx, directory, testLimits())
	if err != nil {
		t.Fatal(err)
	}
	before := peer.count()
	retry, err := importer.ImportPrepared(ctx, reopened.Key, reopened.CandidateID, reopened.Editions)
	if err != nil || !retry.Duplicate || retry.Generation != result.Generation || peer.count() != before {
		t.Fatal(retry, err, "remote delta", peer.count()-before)
	}
	limits := importLimits()
	reader, err := publishedsearch.Open(ctx, cat, objects, publishedsearch.Selection{Collection: "guide", Generation: result.Generation.ID, Edition: "one"}, publishedsearch.Limits{Manifest: limits.Manifest, Edition: limits.Edition.File, Article: limits.Edition.Article})
	if err != nil {
		t.Fatal(err)
	}
	query := termsearch.Query{Edition: "one", TermsVersion: textterms.Version, Terms: []string{"retry"}}
	hits, err := reader.SearchQuoted(ctx, query, termsearch.Limits{MaxEditions: 1, MaxPassages: 1000, MaxTerms: 20000, MaxQueryTerms: 16, MaxInputBytes: 1 << 20, MaxMatches: 32}, publishedsearch.QuoteLimits{MaxQuoteBytes: 4096, MaxTotalQuoteBytes: 8192})
	if err != nil || len(hits.Hits) != 2 || hits.Hits[0].Quote != "Pods\nRetry uploads safely." {
		t.Fatal(hits, err)
	}
	t.Log("producer removed; published documents=2; same-key retry after quarantine: object requests=0; exact quote=Pods\\nRetry uploads safely.")
}
