package gitsource

import (
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fixture struct {
	repo string
	ctx  context.Context
}

func newFixture(t *testing.T, format string) fixture {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)
	f := fixture{repo: t.TempDir(), ctx: ctx}
	f.git(t, "init", "--object-format="+format)
	return f
}

func (f fixture) git(t *testing.T, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(f.ctx, "git", append([]string{"-C", f.repo, "-c", "core.hooksPath="}, args...)...)
	cmd.Env = append(gitEnvironment(), "GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.invalid",
		"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.invalid")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("fixture git %v: %v: %s", args, err, output)
	}
	return strings.TrimSpace(string(output))
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

func (f fixture) commit(t *testing.T) string {
	t.Helper()
	f.git(t, "add", "--all")
	f.git(t, "commit", "-m", "synthetic fixture")
	return f.git(t, "rev-parse", "HEAD")
}

func budgets() Limits {
	return Limits{MaxEntries: 20, MaxListingBytes: 16 << 10, MaxDocumentBytes: 1 << 10, MaxTotalBytes: 4 << 10}
}

func TestCapturePinsCommittedBytesAndOwnsResults(t *testing.T) {
	for _, format := range []string{"sha1", "sha256"} {
		t.Run(format, func(t *testing.T) {
			f := newFixture(t, format)
			f.write(t, "docs/a.md", "old guide\n")
			f.write(t, "docs/nested/space\tline\n.md", "nested guide\n")
			f.write(t, "docs/ignored.MD", "outside exact suffix selection")
			f.write(t, "docs/notes.txt", "not Markdown selection")
			f.write(t, "outside.md", "outside selected directory")
			commit := f.commit(t)
			f.write(t, "docs/a.md", "new committed guide\n")
			newer := f.commit(t)
			f.write(t, "docs/a.md", "uncommitted guide\n")
			f.write(t, "docs/untracked.md", "not in any commit")
			// Replacement refs and inherited environment must not change the pinned
			// object identity or redirect access to a different repository.
			f.git(t, "replace", commit, newer)
			t.Setenv("GIT_DIR", filepath.Join(t.TempDir(), "missing"))
			t.Setenv("GIT_CONFIG_COUNT", "1")
			t.Setenv("GIT_CONFIG_KEY_0", "core.abbrev")
			t.Setenv("GIT_CONFIG_VALUE_0", "7")
			req := Request{Repository: f.repo, Commit: commit, Directory: "docs"}
			got, err := Capture(f.ctx, req, budgets())
			if err != nil {
				t.Fatal(err)
			}
			if got.Commit != commit || got.Directory != "docs" || len(got.Documents) != 2 || got.Ignored != 2 {
				t.Fatalf("wrong scope/count: %+v", got)
			}
			for i, want := range []struct{ name, body string }{
				{"docs/a.md", "old guide\n"},
				{"docs/nested/space\tline\n.md", "nested guide\n"},
			} {
				doc := got.Documents[i]
				if doc.Path != want.name || string(doc.Body) != want.body || doc.SHA256 != sha256.Sum256([]byte(want.body)) || !objectID(doc.BlobID, len(commit)) {
					t.Fatalf("wrong captured document: %+v", doc)
				}
			}
			got.Documents[0].Body[0] = 'X'
			again, err := Capture(f.ctx, req, budgets())
			if err != nil || string(again.Documents[0].Body) != "old guide\n" {
				t.Fatalf("caller mutation changed later capture: %+v, %v", again, err)
			}
		})
	}
}

func TestLiteralDirectoryFromRepositorySubdirectory(t *testing.T) {
	f := newFixture(t, "sha1")
	f.write(t, "docs[1]/guide.md", "literal directory")
	f.write(t, "docs1/guide.md", "must not match")
	commit := f.commit(t)
	got, err := Capture(f.ctx, Request{Repository: filepath.Join(f.repo, "docs[1]"), Commit: commit, Directory: "docs[1]"}, budgets())
	if err != nil || len(got.Documents) != 1 || got.Documents[0].Path != "docs[1]/guide.md" {
		t.Fatalf("literal root/full-tree scope: %+v, %v", got, err)
	}
	got, err = Capture(f.ctx, Request{Repository: f.repo, Commit: commit, Directory: "."}, budgets())
	if err != nil || len(got.Documents) != 2 {
		t.Fatalf("repository-root selection: %+v, %v", got, err)
	}
}

func TestCaptureRejectsLimitsWithoutPartialResults(t *testing.T) {
	f := newFixture(t, "sha1")
	f.write(t, "docs/a.md", "abc")
	f.write(t, "docs/b.md", "defg")
	commit := f.commit(t)
	req := Request{Repository: f.repo, Commit: commit, Directory: "docs"}
	for name, change := range map[string]func(*Limits){
		"entries":  func(l *Limits) { l.MaxEntries = 1 },
		"listing":  func(l *Limits) { l.MaxListingBytes = 1 },
		"document": func(l *Limits) { l.MaxDocumentBytes = 3 },
		"total":    func(l *Limits) { l.MaxDocumentBytes = 4; l.MaxTotalBytes = 6 },
	} {
		t.Run(name, func(t *testing.T) {
			limits := budgets()
			change(&limits)
			got, err := Capture(f.ctx, req, limits)
			if !errors.Is(err, ErrLimit) || got.Commit != "" || got.Documents != nil {
				t.Fatalf("must reject, not truncate: %+v, %v", got, err)
			}
		})
	}
	limits := budgets()
	limits.MaxDocumentBytes, limits.MaxTotalBytes, limits.MaxEntries = 4, 7, 2
	if _, err := Capture(f.ctx, req, limits); err != nil {
		t.Fatal("exact byte/count budgets should fit:", err)
	}
	// Exercise an actual Git subprocess output overflow independently of listing
	// size preflight. run must cancel and wait, returning no truncated output.
	blob := f.git(t, "rev-parse", commit+":docs/b.md")
	if data, err := run(f.ctx, f.repo, 3, "cat-file", "blob", blob); !errors.Is(err, ErrLimit) || data != nil {
		t.Fatalf("subprocess overflow: %q, %v", data, err)
	}
}

func TestUnsupportedEntriesAreExplicit(t *testing.T) {
	for _, kind := range []string{"symlink", "lfs", "submodule"} {
		t.Run(kind, func(t *testing.T) {
			f := newFixture(t, "sha1")
			f.write(t, "docs/a.md", "guide")
			base := f.commit(t)
			switch kind {
			case "symlink":
				if err := os.Symlink("a.md", filepath.Join(f.repo, "docs", "link.md")); err != nil {
					t.Fatal(err)
				}
			case "lfs":
				f.write(t, "docs/pointer.md", "version https://git-lfs.github.com/spec/v1\noid sha256:fixture\nsize 123\n")
			case "submodule":
				f.git(t, "update-index", "--add", "--cacheinfo", "160000,"+base+",docs/sub")
				// Commit the synthetic gitlink without add --all removing it.
				f.git(t, "commit", "-m", "synthetic submodule")
			}
			commit := f.git(t, "rev-parse", "HEAD")
			if kind != "submodule" {
				commit = f.commit(t)
			}
			got, err := Capture(f.ctx, Request{Repository: f.repo, Commit: commit, Directory: "docs"}, budgets())
			if !errors.Is(err, ErrUnsupported) || got.Commit != "" {
				t.Fatalf("unsupported %s must fail explicitly: %+v, %v", kind, got, err)
			}
		})
	}
}

func TestInvalidMissingCanceledAndEmptyCaptures(t *testing.T) {
	f := newFixture(t, "sha1")
	f.write(t, "docs/a.md", "guide")
	f.write(t, "other/readme.txt", "outside selection")
	commit := f.commit(t)
	req := Request{Repository: f.repo, Commit: commit, Directory: "docs"}
	for _, invalid := range []Request{
		{Repository: f.repo, Commit: "HEAD", Directory: "docs"},
		{Repository: f.repo, Commit: commit[:12], Directory: "docs"},
		{Repository: f.repo, Commit: commit, Directory: "../docs"},
		{Repository: f.repo, Commit: commit, Directory: "docs/"},
		{Repository: f.repo, Commit: commit, Directory: "docs/a.md"},
		{Repository: f.repo, Commit: f.git(t, "rev-parse", commit+":docs/a.md"), Directory: "docs"},
	} {
		if _, err := Capture(f.ctx, invalid, budgets()); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid request should fail: %+v: %v", invalid, err)
		}
	}
	if _, err := Capture(context.Background(), req, budgets()); !errors.Is(err, ErrInvalid) {
		t.Fatal("capture without deadline must be rejected:", err)
	}
	canceled, cancel := context.WithCancel(f.ctx)
	cancel()
	if _, err := Capture(canceled, req, budgets()); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation lost:", err)
	}
	missing := req
	missing.Commit = strings.Repeat("0", 40)
	if got, err := Capture(f.ctx, missing, budgets()); err == nil || got.Commit != "" {
		t.Fatalf("missing objects must fail: %+v, %v", got, err)
	}
	empty := req
	empty.Directory = "other"
	if got, err := Capture(f.ctx, empty, budgets()); err != nil || len(got.Documents) != 0 || got.Ignored != 1 {
		t.Fatalf("empty selection is an observation: %+v, %v", got, err)
	}
	blob := f.git(t, "rev-parse", commit+":docs/a.md")
	if err := os.Remove(filepath.Join(f.repo, ".git", "objects", blob[:2], blob[2:])); err != nil {
		t.Fatal(err)
	}
	if got, err := Capture(f.ctx, req, budgets()); err == nil || got.Commit != "" || got.Documents != nil {
		t.Fatalf("missing blob must not yield partial success: %+v, %v", got, err)
	}
}
