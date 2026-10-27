package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sidsharma96/concordance/internal/gitsource"
)

func TestInspectCountsBytesWithoutMergingDocuments(t *testing.T) {
	body := []byte("---\r\ntitle: Example\r\n---\r\n{{< note >}} {{< note >}} ![diagram](x.svg) <table>\n")
	digest := sha256.Sum256(body)
	snapshot := gitsource.Snapshot{Documents: []gitsource.Document{
		{Path: "z.md", Body: body, SHA256: digest},
		{Path: "a.md", Body: body, SHA256: digest},
		{Path: "empty.md", SHA256: sha256.Sum256(nil)},
	}}
	stats := inspect(snapshot)
	if stats.unique != 2 || stats.uniqueBytes != int64(len(body)) || stats.bytes != 2*int64(len(body)) ||
		stats.frontmatter != 2 || stats.shortcodes != 2 || stats.images != 2 || stats.html != 2 || stats.markerDocuments["note"] != 2 {
		t.Fatalf("unexpected inventory: %+v", stats)
	}
	if stats.documents[0].Path != "a.md" || snapshot.Documents[0].Path != "z.md" {
		t.Fatal("sorting changed caller-owned descriptors or failed to order report")
	}
}

func TestMarkersAreNotExtractionCoverage(t *testing.T) {
	// A fenced literal still counts. The tool deliberately does not claim to parse
	// Markdown, resolve includes, or distinguish prose from code/HTML examples.
	body := []byte("```\n{{% example %}}\n```\n{{< /note >}}\n")
	stats := inspect(gitsource.Snapshot{Documents: []gitsource.Document{{Body: body, SHA256: sha256.Sum256(body)}}})
	if stats.shortcodes != 1 || stats.markerDocuments["example"] != 1 || stats.markerDocuments["note"] != 0 {
		t.Fatalf("unexpected marker counts: %+v", stats)
	}
}

func TestRunCommittedCaptureAndFailure(t *testing.T) {
	repo := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		command := exec.Command("git", append([]string{"-C", repo, "-c", "core.hooksPath=/dev/null"}, args...)...)
		command.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + t.TempDir(), "GIT_CONFIG_NOSYSTEM=1"}
		out, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s: %v: %s", args[0], err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "--quiet")
	if err := os.Mkdir(filepath.Join(repo, "docs"), 0o700); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"a.md": "original\n", "b.md": "original\n", "ignored.txt": "other\n"} {
		if err := os.WriteFile(filepath.Join(repo, "docs", name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	git("add", ".")
	git("-c", "user.name=Corpus Test", "-c", "user.email=corpus@example.invalid", "commit", "--quiet", "-m", "fixture")
	commit := git("rev-parse", "HEAD")
	if err := os.WriteFile(filepath.Join(repo, "docs", "a.md"), []byte("uncommitted\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	args := []string{"-repo", repo, "-commit", commit, "-dir", "docs", "-inventory"}
	var first, second bytes.Buffer
	if err := run(t.Context(), args, &first); err != nil {
		t.Fatal(err)
	}
	if err := run(t.Context(), args, &second); err != nil || first.String() != second.String() {
		t.Fatalf("report must be deterministic: %v", err)
	}
	for _, expected := range []string{"documents\t2\n", "ignored_entries\t1\n", "unique_bodies\t1\n", "body_bytes\t18\n", `document\t"docs/a.md"`} {
		expected = strings.ReplaceAll(expected, `\t`, "\t")
		if !strings.Contains(first.String(), expected) {
			t.Fatalf("missing %q in report", expected)
		}
	}
	var failed bytes.Buffer
	if err := run(t.Context(), append(args, "-max-document-bytes", "1"), &failed); err == nil || failed.Len() != 0 {
		t.Fatal("capture limit must fail before reporting")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := run(ctx, args, &failed); !errors.Is(err, context.Canceled) || failed.Len() != 0 {
		t.Fatalf("cancellation: %v", err)
	}
}
