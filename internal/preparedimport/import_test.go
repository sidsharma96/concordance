package preparedimport

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/sidsharma96/concordance/internal/catalog"
	gm "github.com/sidsharma96/concordance/internal/generationmanifest"
	"github.com/sidsharma96/concordance/internal/generationpublish"
	"github.com/sidsharma96/concordance/internal/gitsource"
	"github.com/sidsharma96/concordance/internal/htmlarticle"
	"github.com/sidsharma96/concordance/internal/htmlarticlejson"
	"github.com/sidsharma96/concordance/internal/htmledition"
	"github.com/sidsharma96/concordance/internal/htmlpassage"
	"github.com/sidsharma96/concordance/internal/htmlterms"
	"github.com/sidsharma96/concordance/internal/termsearch"
	"github.com/sidsharma96/concordance/internal/textterms"
)

func testContext(t testing.TB) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)
	return ctx
}
func testLimits() Limits {
	article := htmlarticlejson.Limits{MaxEncodedBytes: 1 << 20, MaxBlocks: 1000, MaxGroups: 1000, MaxStringBytes: 65536, MaxValueBytes: 1 << 20, MaxGroupDepth: 64, MaxHTMLBytes: 1 << 20}
	edition := htmledition.Limits{MaxEncodedBytes: 1 << 20, MaxArticles: 32, MaxArticleBytes: 1 << 20, MaxBlocks: 10000, MaxPassages: 1000, MaxTerms: 20000, MaxPreparedBytes: 1 << 20, MaxKeyBytes: 1 << 20, MaxTermBytes: 1024, MaxVocabulary: 10000, MaxPostings: 20000}
	prep := htmlterms.Limits{Passages: htmlpassage.Limits{MaxBlocks: 1000, MaxGroups: 1000, MaxInputBytes: 1 << 20, MaxPassages: 1000, MaxBodyBytes: 4096, MaxReferences: 10000, MaxContextBytes: 1 << 20}, Field: textterms.Limits{MaxInputBytes: 65536, MaxTerms: 10000, MaxTermBytes: 1024, MaxOutputBytes: 1 << 20}, MaxViewBytes: 1 << 20, MaxTerms: 20000, MaxOutputBytes: 1 << 20}
	return Limits{MaxInputBytes: 8 << 20, MaxStagedBytes: 8 << 20, MaxObjects: 64, MaxHashBytes: 1 << 20,
		Manifest: gm.Limits{MaxEncodedBytes: 1 << 20, MaxSources: 4, MaxEditions: 4, MaxCaptures: 8, MaxFiles: 64, MaxDocuments: 32, MaxImages: 32, MaxSourceFiles: 32, MaxSourceDocuments: 32, MaxStringBytes: 4096, MaxValueBytes: 1 << 18, MaxObjectBytes: 1 << 20, MaxReferencedBytes: 8 << 20},
		Git:      gitsource.Limits{MaxEntries: 64, MaxListingBytes: 1 << 16, MaxDocumentBytes: 1 << 20, MaxTotalBytes: 2 << 20}, Files: gitsource.FileLimits{MaxFiles: 32, MaxListingBytes: 1 << 16, MaxFileBytes: 1 << 20, MaxTotalBytes: 2 << 20},
		Extract: htmlarticle.Limits{MaxInputBytes: 1 << 20, MaxTokenBytes: 65536, MaxTokens: 10000, MaxNodes: 10000, MaxDepth: 64, MaxBlocks: 1000, MaxGroups: 1000, MaxBlockBytes: 65536, MaxOutputBytes: 1 << 20}, Edition: htmledition.BuildLimits{File: edition, Article: article, Preparation: prep}}
}
func publicationLimits() generationpublish.Limits {
	l := testLimits()
	return generationpublish.Limits{MaxObjects: l.MaxObjects, MaxBatchObjects: 2, MaxInputBytes: l.MaxStagedBytes, MaxBatchBytes: 1 << 20, Manifest: l.Manifest, Edition: l.Edition.File, Article: l.Edition.Article}
}
func reference(b []byte) gm.Ref { return gm.Ref{SHA256: sha256.Sum256(b), Bytes: int64(len(b))} }

const markdown = "# Pods\n\nRetry uploads safely.\n"
const figure = `<svg xmlns="http://www.w3.org/2000/svg"><text>Pod</text></svg>`
const include = "kind: Pod\n"
const pageHTML = `<main class="fixture"><h1>Pods</h1><p>Retry uploads safely.</p><img src="/images/pod.svg" alt="Pod diagram"></main>`

type fixture struct {
	ctx          context.Context
	repo, commit string
	sources      []Source
	editions     []Edition
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	f := fixture{ctx: testContext(t), repo: t.TempDir()}
	f.git(t, "init", "--object-format=sha1")
	for name, body := range map[string]string{"docs/a.md": markdown, "docs/b.md": markdown, "docs/notes.txt": "not a document", "static/images/pod.svg": figure, "includes/example.yaml": include} {
		f.write(t, name, body)
	}
	f.git(t, "add", "--all")
	f.git(t, "commit", "-m", "synthetic prepared capture")
	f.commit = f.git(t, "rev-parse", "HEAD")
	f.sources = []Source{{ID: "docs", Repository: "https://example.invalid/docs.git", LocalPath: f.repo}}
	c := Capture{Source: "docs", Revision: f.commit, Directory: "docs", Preparation: "synthetic/v1", Toolchain: "no renderer: authored fixture/v1", Recipe: []byte("fixture/v1; producer=authored test; outputs=one per Markdown; print excluded"), Dependencies: []string{"static/images/pod.svg", "includes/example.yaml"}, Profile: htmlarticle.Profile{ID: "fixture/v1", ArticleClass: "fixture"}, Pages: []Page{{"docs/b.md", []byte(pageHTML)}, {"docs/a.md", []byte(pageHTML)}}}
	f.editions = []Edition{{ID: "one", Captures: []Capture{c}}}
	return f
}
func (f fixture) git(t *testing.T, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(f.ctx, "git", append([]string{"--no-replace-objects", "-C", f.repo, "-c", "core.hooksPath="}, args...)...)
	// Fixtures must not inherit the operator's Git routing, hooks or identity.
	for _, v := range os.Environ() {
		if !strings.HasPrefix(v, "GIT_") {
			cmd.Env = append(cmd.Env, v)
		}
	}
	cmd.Env = append(cmd.Env, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.invalid", "GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.invalid")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("fixture git: %v: %s", err, out)
	}
	return strings.TrimSpace(string(out))
}
func (f fixture) write(t *testing.T, name, body string) {
	t.Helper()
	name = filepath.Join(f.repo, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(name), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}
func (f fixture) importer(t *testing.T, l Limits) *Importer {
	t.Helper()
	i, err := New(&generationpublish.Publisher{}, "guide", f.sources, l)
	if err != nil {
		t.Fatal(err)
	}
	return i
}
func (f fixture) clone() []Edition {
	out := slices.Clone(f.editions)
	for ei := range out {
		out[ei].Captures = slices.Clone(out[ei].Captures)
		for ci := range out[ei].Captures {
			c := &out[ei].Captures[ci]
			c.Recipe = bytes.Clone(c.Recipe)
			c.Dependencies = slices.Clone(c.Dependencies)
			c.Profile = ownedProfile(c.Profile)
			c.Pages = slices.Clone(c.Pages)
			for pi := range c.Pages {
				c.Pages[pi].HTML = bytes.Clone(c.Pages[pi].HTML)
			}
		}
	}
	return out
}
func opened(t testing.TB, ctx context.Context, b bundle) *gm.Manifest {
	t.Helper()
	m, err := gm.Open(ctx, b.manifest, reference(b.manifest), "guide", "candidate", testLimits().Manifest)
	if err != nil {
		t.Fatal(err)
	}
	return m
}
func artifact(t testing.TB, b bundle, r gm.Ref) []byte {
	t.Helper()
	for _, body := range b.artifacts {
		if reference(body) == r {
			return body
		}
	}
	t.Fatal("required artifact missing")
	return nil
}

func TestPreparedCaptureUsesCompletePinnedGitAndOwnProcessing(t *testing.T) {
	f := newFixture(t)
	f.write(t, "docs/a.md", "uncommitted replacement")
	f.write(t, "docs/untracked.md", "not selected")
	before := f.clone()
	b, err := f.importer(t, testLimits()).build(f.ctx, "candidate", f.editions)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, f.editions) {
		t.Fatal("request mutated")
	}
	m := opened(t, f.ctx, b)
	d, err := m.Describe(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	e := d.Editions[0]
	if len(e.Documents) != 2 || e.Documents[0].NativeID != "docs/a.md" || e.Documents[1].NativeID != "docs/b.md" {
		t.Fatal("wrong source membership/order", e.Documents)
	}
	if e.Documents[0].Article != e.Documents[1].Article || e.Documents[0].HTML != e.Documents[1].HTML {
		t.Fatal("shared bytes lost")
	}
	for slot := range e.Documents {
		binding, err := m.Resolve(f.ctx, "one", slot)
		if err != nil {
			t.Fatal(err)
		}
		if binding.Revision != f.commit || string(artifact(t, b, binding.Input.Object)) != markdown || len(binding.Images) != 1 || string(artifact(t, b, binding.Images[0].Input.Object)) != figure {
			t.Fatal("wrong pinned source/image", binding)
		}
		if _, err := m.VerifyArticle(f.ctx, "one", slot, artifact(t, b, binding.Article), testLimits().Edition.Article); err != nil {
			t.Fatal(err)
		}
	}
	required, err := m.Objects(f.ctx)
	if err != nil || len(required) != len(b.artifacts) {
		t.Fatal("artifact inventory", err)
	}
	for _, r := range required {
		_ = artifact(t, b, r)
	}
	index, err := m.OpenEdition(f.ctx, "one", artifact(t, b, e.Index), testLimits().Edition.File)
	if err != nil {
		t.Fatal(err)
	}
	hits, err := index.Search(f.ctx, termsearch.Query{TermsVersion: textterms.Version, Edition: "one", Terms: []string{"retry"}}, searchLimits())
	if err != nil || len(hits.Matches) != 2 || hits.Matches[0].Passage >= hits.Matches[1].Passage {
		t.Fatal(hits, err)
	}
	t.Logf("selected Git documents=%d; mapped HTML pages=%d; shared article objects=1", len(e.Documents), len(f.editions[0].Captures[0].Pages))
}
func searchLimits() termsearch.Limits {
	return termsearch.Limits{MaxEditions: 1, MaxPassages: 1000, MaxTerms: 20000, MaxQueryTerms: 16, MaxInputBytes: 1 << 20, MaxMatches: 1000}
}

func TestPreparedMembershipRejectsIncompleteOrInvalidInput(t *testing.T) {
	f := newFixture(t)
	tests := []struct {
		name   string
		change func(*Capture)
		want   error
	}{
		{"missing page", func(c *Capture) { c.Pages = c.Pages[:1] }, ErrMembership},
		{"extra page", func(c *Capture) { c.Pages = append(c.Pages, Page{"docs/extra.md", []byte(pageHTML)}) }, ErrMembership},
		{"duplicate page", func(c *Capture) { c.Pages[1].Path = c.Pages[0].Path }, ErrMembership},
		{"wrong path", func(c *Capture) { c.Pages[1].Path = "docs/not-selected.md" }, ErrMembership},
		{"unknown source", func(c *Capture) { c.Source = "unknown" }, ErrInvalid},
		{"mutable revision", func(c *Capture) { c.Revision = "main" }, gitsource.ErrInvalid},
		{"duplicate dependency", func(c *Capture) { c.Dependencies = append(c.Dependencies, c.Dependencies[0]) }, ErrInvalid},
		{"document dependency overlap", func(c *Capture) { c.Dependencies = append(c.Dependencies, "docs/a.md") }, ErrInvalid},
		{"missing dependency", func(c *Capture) { c.Dependencies = append(c.Dependencies, "missing.yaml") }, gitsource.ErrMissing},
		{"undeclared image", func(c *Capture) { c.Dependencies = c.Dependencies[1:] }, gitsource.ErrMissing},
		{"external image", func(c *Capture) {
			c.Pages[0].HTML = []byte(strings.ReplaceAll(pageHTML, "/images/pod.svg", "https://example.invalid/pod.svg"))
		}, gm.ErrUnsupported},
		{"broken selection", func(c *Capture) { c.Pages[0].HTML = []byte("<p>No selected article</p>") }, htmlarticle.ErrSelection},
		{"invalid UTF8", func(c *Capture) { c.Pages[0].HTML = []byte{0xff} }, htmlarticle.ErrInvalid},
		{"missing recipe", func(c *Capture) { c.Recipe = nil }, ErrInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			editions := f.clone()
			tt.change(&editions[0].Captures[0])
			b, err := f.importer(t, testLimits()).build(f.ctx, "candidate", editions)
			if !errors.Is(err, tt.want) || !reflect.DeepEqual(b, bundle{}) {
				t.Fatal(b, err)
			}
			t.Logf("rejected: %s", tt.name)
		})
	}
}

func TestPreparedOrderingSourcesEditionsAndEmptySelection(t *testing.T) {
	f := newFixture(t)
	i := f.importer(t, testLimits())
	first, err := i.build(f.ctx, "candidate", f.editions)
	if err != nil {
		t.Fatal(err)
	}
	reverse := f.clone()
	slices.Reverse(reverse[0].Captures[0].Pages)
	slices.Reverse(reverse[0].Captures[0].Dependencies)
	second, err := i.build(f.ctx, "candidate", reverse)
	if err != nil || !bytes.Equal(first.manifest, second.manifest) {
		t.Fatal("mapping order affected slots", err)
	}
	// The same logical path is valid across editions and sources.
	f.sources = append(f.sources, Source{ID: "another", Repository: "https://example.invalid/another.git", LocalPath: f.repo})
	editions := f.clone()
	another := f.clone()[0].Captures[0]
	another.Source = "another"
	editions[0].Captures = append(editions[0].Captures, another)
	old := f.clone()[0]
	old.ID = "old"
	editions = append(editions, old, Edition{ID: "empty"})
	f.write(t, "empty/notes.txt", "known empty directory")
	f.git(t, "add", "--all")
	f.git(t, "commit", "-m", "empty selection")
	empty := f.clone()[0].Captures[0]
	empty.Revision = f.git(t, "rev-parse", "HEAD")
	empty.Directory = "empty"
	empty.Pages = nil
	empty.Dependencies = nil
	editions = append(editions, Edition{ID: "captured-empty", Captures: []Capture{empty}})
	b, err := f.importer(t, testLimits()).build(f.ctx, "candidate", editions)
	if err != nil {
		t.Fatal(err)
	}
	m := opened(t, f.ctx, b)
	d, err := m.Describe(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Editions[0].Documents) != 4 || d.Editions[0].Captures[0].Source != "another" {
		t.Fatal("source ordering/membership")
	}
	for _, e := range d.Editions[2:] {
		x, err := m.OpenEdition(f.ctx, e.ID, artifact(t, b, e.Index), testLimits().Edition.File)
		if err != nil || x.ArticleCount() != 0 {
			t.Fatal("known empty", err)
		}
	}
	if _, err := m.OpenEdition(f.ctx, "absent", nil, testLimits().Edition.File); !errors.Is(err, gm.ErrNotFound) {
		t.Fatal(err)
	}
	if _, err := m.InputFingerprint(f.ctx, b.rules.Input, b.rules.MaxHashBytes); err != nil {
		t.Fatal(err)
	}
}

func TestPreparedAggregateLimitsAndNoPartialBundle(t *testing.T) {
	f := newFixture(t)
	l := testLimits()
	b, err := f.importer(t, l).build(f.ctx, "candidate", f.editions)
	if err != nil {
		t.Fatal(err)
	}
	staged := int64(len(b.manifest))
	for _, body := range b.artifacts {
		staged += int64(len(body))
	}
	c := f.editions[0].Captures[0]
	input := int64(len(c.Recipe) + 2*len(pageHTML) + 2*len(markdown) + len(figure) + len(include))
	l.MaxStagedBytes = staged
	l.MaxObjects = len(b.artifacts) + 1
	l.MaxInputBytes = input
	if _, err := f.importer(t, l).build(f.ctx, "candidate", f.editions); err != nil {
		t.Fatal("exact budgets", err)
	}
	tests := []struct {
		name   string
		change func(*Limits)
	}{
		{"input", func(l *Limits) { l.MaxInputBytes-- }}, {"staged", func(l *Limits) { l.MaxStagedBytes-- }}, {"objects", func(l *Limits) { l.MaxObjects-- }},
		{"documents", func(l *Limits) { l.Manifest.MaxDocuments = 1 }}, {"source documents", func(l *Limits) { l.Manifest.MaxSourceDocuments = 1 }}, {"files", func(l *Limits) { l.Manifest.MaxFiles = 3 }},
		{"images", func(l *Limits) { l.Manifest.MaxImages = 1 }}, {"metadata", func(l *Limits) { l.Manifest.MaxValueBytes = 300 }}, {"extraction", func(l *Limits) { l.Extract.MaxNodes = 1 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			small := l
			tt.change(&small)
			out, err := f.importer(t, small).build(f.ctx, "candidate", f.editions)
			if err == nil || !reflect.DeepEqual(out, bundle{}) {
				t.Fatal(out, err)
			}
		})
	}
}

func TestPreparedProfilesAdmissionAndLifetimes(t *testing.T) {
	f := newFixture(t)
	i := f.importer(t, testLimits())
	f.sources[0].LocalPath = "caller changed registration"
	// New copied registration; caller owns future mutations of its slice.
	if _, err := i.build(f.ctx, "candidate", f.editions); err != nil {
		t.Fatal(err)
	}
	duplicates := f.clone()
	duplicates = append(duplicates, duplicates[0])
	if _, err := i.build(f.ctx, "candidate", duplicates); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	duplicates = f.clone()
	duplicates[0].Captures = append(duplicates[0].Captures, duplicates[0].Captures[0])
	if _, err := i.build(f.ctx, "candidate", duplicates); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	profiles := f.clone()
	other := f.clone()[0]
	other.ID = "other"
	other.Captures[0].Profile.OmitClasses = []string{"chrome"}
	profiles = append(profiles, other)
	if _, err := i.build(f.ctx, "candidate", profiles); !errors.Is(err, ErrInvalid) {
		t.Fatal("conflicting profile policies", err)
	}
	ctx, cancel := context.WithCancel(f.ctx)
	cancel()
	if _, err := i.ImportPrepared(ctx, "key", "candidate", f.editions); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	for _, ctx := range []context.Context{nil, context.Background()} {
		if _, err := i.ImportPrepared(ctx, "key", "candidate", f.editions); !errors.Is(err, ErrInvalid) {
			t.Fatal(err)
		}
	}
	i.permit <- struct{}{}
	out, err := i.ImportPrepared(f.ctx, "key", "candidate", f.editions)
	<-i.permit
	if !errors.Is(err, ErrBusy) || out != (catalog.Publication{}) {
		t.Fatal(out, err)
	}
	var absent *Importer
	if _, err := absent.ImportPrepared(f.ctx, "key", "candidate", nil); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
}
