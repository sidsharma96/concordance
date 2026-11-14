package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/sidsharma96/concordance/internal/capturefiles"
	"github.com/sidsharma96/concordance/internal/catalog"
	"github.com/sidsharma96/concordance/internal/htmlarticle"
	"github.com/sidsharma96/concordance/internal/importjob"
	"github.com/sidsharma96/concordance/internal/publishedsearch"
	"github.com/sidsharma96/concordance/internal/termsearch"
	"github.com/sidsharma96/concordance/internal/textterms"
)

func importDefaults() importOptions {
	return importOptions{timeout: 10 * time.Minute, input: 512 << 20, staged: 256 << 20, payload: 64 << 20, object: 8 << 20, documents: 1000, files: 20000, concurrent: 1, passages: 20000, vocabulary: 20000}
}

func TestImportEditionCeilingFlags(t *testing.T) {
	f := flag.NewFlagSet("import", flag.ContinueOnError)
	var o importOptions
	o.flags(f)
	if err := f.Parse(nil); err != nil || o != importDefaults() {
		t.Fatalf("defaults changed: %+v %v", o, err)
	}
	if err := f.Parse([]string{"--max-passages", "40000", "--max-vocabulary", "30000"}); err != nil || !o.valid() {
		t.Fatal(o, err)
	}
	if e := o.importerLimits().Edition.File; e.MaxPassages != 40000 || e.MaxVocabulary != 30000 {
		t.Fatalf("edition ceilings not applied: %+v", e)
	}
	for _, edit := range []func(*importOptions){func(o *importOptions) { o.passages = 0 }, func(o *importOptions) { o.vocabulary = -1 }} {
		bad := importDefaults()
		edit(&bad)
		if bad.valid() {
			t.Fatalf("nonpositive edition ceiling accepted: %+v", bad)
		}
	}
}

func gitFixture(t *testing.T) (string, func(...string) string) {
	t.Helper()
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, "docs"), 0700); err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) string {
		cmd := exec.CommandContext(testContext(t), "git", append([]string{"-C", repo, "-c", "core.hooksPath=", "-c", "commit.gpgSign=false"}, args...)...)
		for _, s := range os.Environ() {
			if !strings.HasPrefix(s, "GIT_") {
				cmd.Env = append(cmd.Env, s)
			}
		}
		b, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatal("synthetic Git failed", err)
		}
		return strings.TrimSpace(string(b))
	}
	git("init", "--quiet")
	git("config", "user.name", "Fixture")
	git("config", "user.email", "fixture@example.invalid")
	return repo, git
}

func newCapture(t *testing.T, repo string, git func(...string) string, text string) string {
	t.Helper()
	for _, name := range []string{"a.md", "b.md"} {
		if err := os.WriteFile(filepath.Join(repo, "docs", name), []byte("# Pods\n"+text+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	git("add", ".")
	git("commit", "--quiet", "-m", "fixture update")
	dir := t.TempDir()
	html := []byte("<main class=\"fixture\"><h1>Pods</h1><p>" + text + "</p></main>")
	recipe := []byte("authored fixture/v1; print outputs excluded")
	ref := func(path string, b []byte) capturefiles.File {
		return capturefiles.File{Path: path, Bytes: int64(len(b)), SHA256: sha256.Sum256(b)}
	}
	d := capturefiles.Description{Editions: []capturefiles.Edition{
		{ID: "one", Captures: []capturefiles.Capture{{Source: "docs", Revision: git("rev-parse", "HEAD"), Directory: "docs", Preparation: "authored/v1", Toolchain: "fixture/v1", Recipe: ref("recipe", recipe),
			Dependencies: []string{}, Profile: htmlarticle.Profile{ID: "fixture/v1", ArticleClass: "fixture"},
			Pages: []capturefiles.Page{{Path: "docs/b.md", HTML: ref("html", html)}, {Path: "docs/a.md", HTML: ref("html", html)}}}}},
		{ID: "empty", Captures: []capturefiles.Capture{}},
	}}
	b, err := capturefiles.Encode(testContext(t), d, importDefaults().jobLimits().Capture)
	if err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string][]byte{capturefiles.DescriptorName: b, "recipe": recipe, "html": html} {
		if err := os.WriteFile(filepath.Join(dir, name), body, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func writeSources(t *testing.T, repo string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "sources")
	if err := os.WriteFile(p, profileBytes(repo), 0600); err != nil {
		t.Fatal(err)
	}
	return p
}

// Every command gets a newly constructed pool/catalog/client, not a retained
// namespace or importer. Only the controlled HTTP peer and DB schema survive.
func freshImportConnector(t *testing.T, f fixture, opened, closed *int) searchConnector {
	t.Helper()
	return func(ctx context.Context, c searchConfig, _ int64) (searchStores, error) {
		*opened++
		pool, err := pgxpool.NewWithConfig(ctx, f.pool.Config())
		if err != nil {
			return searchStores{}, err
		}
		cat, err := catalog.New(pool)
		if err != nil {
			pool.Close()
			return searchStores{}, err
		}
		return searchStores{catalog: cat, objects: f.newObjects(t, c.collection), close: func() { pool.Close(); *closed++ }}, nil
	}
}

func TestImportCommandStartActivateUpdateSearchAndSwitchback(t *testing.T) {
	f := setup(t)
	ctx := testContext(t)
	repo, git := gitFixture(t)
	profile := writeSources(t, repo)
	producer := newCapture(t, repo, git, "Retry uploads safely.")
	job := filepath.Join(t.TempDir(), "job")
	opened, closed := 0, 0
	connect := freshImportConnector(t, f, &opened, &closed)
	var out bytes.Buffer
	start := []string{"start", "--capture", producer, "--job", job, "--sources", profile}
	if err := runImport(ctx, start, &out, environment(configEnvironment()), connect); err != nil {
		t.Fatal(out.String(), err)
	}
	saved, err := importjob.Open(ctx, job, importDefaults().jobLimits())
	if err != nil {
		t.Fatal(err)
	}
	first := saved.CandidateID
	if !strings.Contains(out.String(), "Published generation: "+first) || !strings.Contains(out.String(), "Recovered existing result: false") {
		t.Fatal(out.String())
	}
	if _, err := f.cat.ResolveTarget(ctx, "kubernetes"); !errors.Is(err, catalog.ErrNotFound) {
		t.Fatal("import activated target", err)
	}
	activate := func(generation string, expected int64) {
		t.Helper()
		out.Reset()
		if err := runTarget(ctx, []string{"activate", "--generation", generation, "--edition", "one", "--expected-revision", strconv.FormatInt(expected, 10), "kubernetes"}, &out, environment(configEnvironment()), connect); err != nil {
			t.Fatal(err)
		}
	}
	activate(first, 0)
	old, err := publishedsearch.Open(ctx, f.cat, f.objects, publishedsearch.Selection{Collection: "guide", Generation: first, Edition: "one"}, testLimits())
	if err != nil {
		t.Fatal(err)
	}
	// Updating a target must not redirect an already-open exact-generation reader.
	secondProducer := newCapture(t, repo, git, "Restart containers deliberately.")
	secondJob := filepath.Join(t.TempDir(), "job")
	out.Reset()
	if err := runImport(ctx, []string{"start", "--capture", secondProducer, "--job", secondJob, "--sources", profile}, &out, environment(configEnvironment()), connect); err != nil {
		t.Fatal(err)
	}
	second, err := importjob.Open(ctx, secondJob, importDefaults().jobLimits())
	if err != nil {
		t.Fatal(err)
	}
	target, err := f.cat.ResolveTarget(ctx, "kubernetes")
	if err != nil || target.Value.Generation != first || target.Revision != 1 {
		t.Fatal(target, err)
	}
	activate(second.CandidateID, 1)
	out.Reset()
	if err := runSearch(ctx, []string{"kubernetes", "restart"}, &out, environment(configEnvironment()), connect); err != nil || !strings.Contains(out.String(), "Restart containers deliberately.") {
		t.Fatal(out.String(), err)
	}
	q := termsearch.Query{Edition: "one", TermsVersion: textterms.Version, Terms: []string{"retry"}}
	result, err := old.SearchQuoted(ctx, q, queryLimits(), publishedsearch.QuoteLimits{MaxQuoteBytes: 4096, MaxTotalQuoteBytes: 8192})
	if err != nil || len(result.Hits) != 2 || result.Hits[0].Quote != "Pods\nRetry uploads safely." {
		t.Fatal(result, err)
	}
	activate(first, 2)
	out.Reset()
	if err := runSearch(ctx, []string{"kubernetes", "retry"}, &out, environment(configEnvironment()), connect); err != nil || !strings.Contains(out.String(), "Retry uploads safely.") {
		t.Fatal(out.String(), err)
	}
	// Producer is unnecessary for retry; Git pinned commits remain available.
	if err := os.RemoveAll(producer); err != nil {
		t.Fatal(err)
	}
	if err := f.cat.Quarantine(ctx, f.namespace); err != nil {
		t.Fatal(err)
	}
	f.remote.reset()
	out.Reset()
	if err := runImport(ctx, []string{"retry", "--job", job, "--sources", profile}, &out, environment(configEnvironment()), connect); err != nil {
		t.Fatal(err)
	}
	if len(f.remote.counts()) != 0 || !strings.Contains(out.String(), first) || !strings.Contains(out.String(), "Recovered existing result: true") {
		t.Fatal(out.String(), f.remote.counts())
	}
	target, err = f.cat.ResolveTarget(ctx, "kubernetes")
	if err != nil || target.Value.Generation != first || target.Revision != 3 {
		t.Fatal(target, err)
	}
	// New work under quarantine may bind its key, but gets no object access and
	// cannot publish. A later operator recovery protocol remains separate.
	blockedJob := filepath.Join(t.TempDir(), "job")
	out.Reset()
	err = runImport(ctx, []string{"start", "--capture", secondProducer, "--job", blockedJob, "--sources", profile}, &out, environment(configEnvironment()), connect)
	if !errors.Is(err, catalog.ErrNamespace) || out.Len() != 0 || len(f.remote.counts()) != 0 {
		t.Fatal(err, out.String(), f.remote.counts())
	}
	blocked, err := importjob.Open(ctx, blockedJob, importDefaults().jobLimits())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.cat.LookupGeneration(ctx, "guide", blocked.CandidateID); !errors.Is(err, catalog.ErrNotFound) {
		t.Fatal(err)
	}
	if opened != closed {
		t.Fatal("client ownership", opened, closed)
	}
	t.Log("start -> explicit activation -> search -> update -> old reader stays pinned -> switch back; fresh-client quarantined retry returns original generation with zero object requests")
}

func TestImportCommandOutputFailureRecoversAndStartNeverOverwrites(t *testing.T) {
	f := setup(t)
	repo, git := gitFixture(t)
	producer := newCapture(t, repo, git, "Retry uploads safely.")
	profile := writeSources(t, repo)
	job := filepath.Join(t.TempDir(), "job")
	opened, closed := 0, 0
	connect := freshImportConnector(t, f, &opened, &closed)
	args := []string{"start", "--capture", producer, "--job", job, "--sources", profile}
	err := runImport(testContext(t), args, shortOutput{}, environment(configEnvironment()), connect)
	if !errors.Is(err, io.ErrShortWrite) || !strings.Contains(err.Error(), "publication completed but output failed") {
		t.Fatal(err)
	}
	saved, err := importjob.Open(testContext(t), job, importDefaults().jobLimits())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.cat.LookupGeneration(testContext(t), "guide", saved.CandidateID); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	before := opened
	f.remote.reset()
	if err := runImport(testContext(t), args, &out, environment(configEnvironment()), connect); !errors.Is(err, os.ErrExist) || out.Len() != 0 || opened != before || len(f.remote.counts()) != 0 {
		t.Fatal(err, out.String())
	}
	reopened, err := importjob.Open(testContext(t), job, importDefaults().jobLimits())
	if err != nil || reopened.Key != saved.Key {
		t.Fatal(reopened, err)
	}
	if err := os.RemoveAll(producer); err != nil {
		t.Fatal(err)
	}
	if err := runImport(testContext(t), []string{"retry", "--job", job, "--sources", profile}, &out, environment(configEnvironment()), connect); err != nil || !strings.Contains(out.String(), saved.CandidateID) || len(f.remote.counts()) != 0 {
		t.Fatal(err, out.String(), f.remote.counts())
	}
	if opened != closed {
		t.Fatal("client leak", opened, closed)
	}
	t.Log("output failed after publication; start refused existing job; explicit retry recovered original generation without producer or object requests")
}

func TestImportAdmissionAndSafeFailureBeforeConnection(t *testing.T) {
	ctx := testContext(t)
	connect := func(context.Context, searchConfig, int64) (searchStores, error) {
		t.Fatal("unexpected connection")
		return searchStores{}, nil
	}
	for _, args := range [][]string{
		{"other"}, {"retry"}, {"start", "--job", "new", "--sources", "profile"}, {"retry", "--capture", "input", "--job", "old", "--sources", "profile"},
		{"retry", "--job", "old", "--sources", "profile", "--object-concurrency", "9"},
		{"retry", "--job", "old", "--sources", "profile", "--max-documents", "0"},
		{"retry", "--job", "old", "--sources", "profile", "--max-object-bytes", "1"},
		{"retry", "--job", "old", "--sources", "profile", "--timeout", "0"},
		{"retry", "--private-capture-do-not-echo"},
	} {
		var out bytes.Buffer
		if err := runImport(ctx, args, &out, environment(configEnvironment()), connect); err == nil || strings.Contains(err.Error(), "private-capture") || out.Len() != 0 {
			t.Fatal(args, err, out.String())
		}
	}
	repo, git := gitFixture(t)
	producer := newCapture(t, repo, git, "Retry uploads safely.")
	profile := writeSources(t, repo)
	job := filepath.Join(t.TempDir(), "job")
	sources, err := readSources(ctx, profile)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := importjob.Prepare(ctx, producer, job, jobRequest("guide", sources), importDefaults().jobLimits()); err != nil {
		t.Fatal(err)
	}
	body := profileBytes(repo)
	body = bytes.ReplaceAll(body, []byte("https://example.invalid/docs.git"), []byte("https://example.invalid/wrong.git"))
	if err := os.WriteFile(profile, body, 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := runImport(ctx, []string{"retry", "--job", job, "--sources", profile}, &out, environment(configEnvironment()), connect); !errors.Is(err, importjob.ErrMismatch) || out.Len() != 0 {
		t.Fatal(err)
	}
	err = importFailure("stage", "job\n\x1b[31m", fmt.Errorf("secret-value: %w", os.ErrNotExist))
	if strings.Contains(err.Error(), "secret-value") || strings.Contains(err.Error(), "worker socket") || strings.ContainsAny(err.Error(), "\n\x1b") {
		t.Fatal(err)
	}
}

func TestImportCommandRetriesUnfinishedPublicationWithSameJob(t *testing.T) {
	f := setup(t)
	repo, git := gitFixture(t)
	producer := newCapture(t, repo, git, "Retry uploads safely.")
	profile := writeSources(t, repo)
	job := filepath.Join(t.TempDir(), "job")
	opened, closed := 0, 0
	connect := freshImportConnector(t, f, &opened, &closed)
	f.remote.mu.Lock()
	f.remote.denyPUT = true
	f.remote.mu.Unlock()
	var out bytes.Buffer
	err := runImport(testContext(t), []string{"start", "--capture", producer, "--job", job, "--sources", profile}, &out, environment(configEnvironment()), connect)
	if err == nil || out.Len() != 0 {
		t.Fatal(err, out.String())
	}
	saved, err := importjob.Open(testContext(t), job, importDefaults().jobLimits())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.cat.LookupGeneration(testContext(t), "guide", saved.CandidateID); !errors.Is(err, catalog.ErrNotFound) {
		t.Fatal("partial publication", err)
	}
	if err := os.RemoveAll(producer); err != nil {
		t.Fatal(err)
	}
	f.remote.mu.Lock()
	f.remote.denyPUT = false
	f.remote.mu.Unlock()
	out.Reset()
	if err := runImport(testContext(t), []string{"retry", "--job", job, "--sources", profile}, &out, environment(configEnvironment()), connect); err != nil || !strings.Contains(out.String(), saved.CandidateID) {
		t.Fatal(err, out.String())
	}
	again, err := importjob.Open(testContext(t), job, importDefaults().jobLimits())
	if err != nil || again.Key != saved.Key {
		t.Fatal(again, err)
	}
	if opened != closed {
		t.Fatal("client leak", opened, closed)
	}
	t.Log("provider refused creation; no partial generation; saved job survived; explicit retry completed original key after producer removal")
}

func TestImportCommandPreservesJobOnMissingNamespaceAndConnectionFailure(t *testing.T) {
	f := setup(t)
	repo, git := gitFixture(t)
	producer := newCapture(t, repo, git, "Retry uploads safely.")
	profile := writeSources(t, repo)
	job := filepath.Join(t.TempDir(), "job")
	var out bytes.Buffer
	secret := errors.New("private connection diagnostic with password")
	broken := func(context.Context, searchConfig, int64) (searchStores, error) { return searchStores{}, secret }
	err := runImport(testContext(t), []string{"start", "--capture", producer, "--job", job, "--sources", profile}, &out, environment(configEnvironment()), broken)
	if !errors.Is(err, secret) || strings.Contains(err.Error(), "password") || out.Len() != 0 {
		t.Fatal(err)
	}
	saved, err := importjob.Open(testContext(t), job, importDefaults().jobLimits())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(testContext(t), "DELETE FROM concordance_object_namespaces"); err != nil {
		t.Fatal(err)
	}
	opened, closed := 0, 0
	connect := freshImportConnector(t, f, &opened, &closed)
	f.remote.reset()
	err = runImport(testContext(t), []string{"retry", "--job", job, "--sources", profile}, &out, environment(configEnvironment()), connect)
	if !errors.Is(err, catalog.ErrNotFound) || out.Len() != 0 || len(f.remote.counts()) != 0 {
		t.Fatal(err)
	}
	var count int
	if err := f.pool.QueryRow(testContext(t), "SELECT count(*) FROM concordance_object_namespaces").Scan(&count); err != nil || count != 0 {
		t.Fatal(count, err)
	}
	reopened, err := importjob.Open(testContext(t), job, importDefaults().jobLimits())
	if err != nil || reopened.Key != saved.Key {
		t.Fatal(reopened, err)
	}
	if opened != closed {
		t.Fatal("client leak", opened, closed)
	}
}

func TestImportCommandPublishesKnownEmptySnapshot(t *testing.T) {
	f := setup(t)
	profile := filepath.Join(t.TempDir(), "sources")
	if err := os.WriteFile(profile, []byte("{\"format\":\""+sourcesFormat+"\",\"sources\":[]}"), 0600); err != nil {
		t.Fatal(err)
	}
	producer := t.TempDir()
	desc, err := capturefiles.Encode(testContext(t), capturefiles.Description{Editions: []capturefiles.Edition{{ID: "empty", Captures: []capturefiles.Capture{}}}}, importDefaults().jobLimits().Capture)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(producer, capturefiles.DescriptorName), desc, 0600); err != nil {
		t.Fatal(err)
	}
	job := filepath.Join(t.TempDir(), "job")
	opened, closed := 0, 0
	connect := freshImportConnector(t, f, &opened, &closed)
	var out bytes.Buffer
	if err := runImport(testContext(t), []string{"start", "--capture", producer, "--job", job, "--sources", profile}, &out, environment(configEnvironment()), connect); err != nil {
		t.Fatal(err)
	}
	saved, err := importjob.Open(testContext(t), job, importDefaults().jobLimits())
	if err != nil {
		t.Fatal(err)
	}
	reader, err := publishedsearch.Open(testContext(t), f.cat, f.objects, publishedsearch.Selection{Collection: "guide", Generation: saved.CandidateID, Edition: "empty"}, testLimits())
	if err != nil {
		t.Fatal(err)
	}
	result, err := reader.Search(testContext(t), termsearch.Query{Edition: "empty", TermsVersion: textterms.Version, Terms: []string{"retry"}}, queryLimits())
	if err != nil || len(result.Hits) != 0 {
		t.Fatal(result, err)
	}
	if opened != closed {
		t.Fatal("client leak")
	}
}
