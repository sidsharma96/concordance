package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sidsharma96/concordance/internal/gitsource"
	"github.com/sidsharma96/concordance/internal/htmlarticle"
)

func TestImageAddressBoundary(t *testing.T) {
	got, err := imagePath("/images/docs/pod.svg")
	if err != nil || got != "static/images/docs/pod.svg" {
		t.Fatal(got, err)
	}
	for _, address := range []string{"", "/images/", "/figure.svg", "images/pod.svg", "../images/pod.svg", "https://example.invalid/a.svg", "data:image/svg+xml,a", "//images/pod.svg", "/images/../secret", "/images/a/./b", "/images//x", "/images/a?x=1", "/images/a#x", "/images/%2e%2e/secret", "/images/a\\b", "/images/a\x00b", "/images/a\nb", "/images/\xff"} {
		if _, err := imagePath(address); err == nil {
			t.Fatalf("accepted unsupported address %q", address)
		}
	}
	a := htmlarticle.Article{Blocks: []htmlarticle.Block{{Kind: htmlarticle.Image, SourceURL: "/images/b.svg"}, {Kind: htmlarticle.Image, SourceURL: "/images/a.svg"}, {Kind: htmlarticle.Image, SourceURL: "/images/b.svg"}}}
	paths, err := imagePaths(a)
	if err != nil || strings.Join(paths, ",") != "static/images/a.svg,static/images/b.svg" || len(a.Blocks) != 3 {
		t.Fatal("dedup lost association or order", paths, err)
	}
	a.Blocks = make([]htmlarticle.Block, 129)
	for i := range a.Blocks {
		a.Blocks[i] = htmlarticle.Block{Kind: htmlarticle.Image, SourceURL: fmt.Sprintf("/images/%d.svg", i)}
	}
	if _, err := imagePaths(a); !errors.Is(err, gitsource.ErrLimit) {
		t.Fatal("unique asset budget missing", err)
	}
	a.Blocks = make([]htmlarticle.Block, 257)
	for i := range a.Blocks {
		a.Blocks[i] = htmlarticle.Block{Kind: htmlarticle.Image, SourceURL: "/images/a.svg"}
	}
	if _, err := imagePaths(a); !errors.Is(err, gitsource.ErrLimit) {
		t.Fatal("occurrence budget lost after dedup", err)
	}
}

func TestAuditCapturesPinnedImagesOrNoReport(t *testing.T) {
	repo := t.TempDir()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.CommandContext(ctx, "git", append([]string{"-C", repo, "-c", "core.hooksPath="}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.invalid", "GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.invalid")
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("fixture git failed: %v %s", err, output)
		}
		return strings.TrimSpace(string(output))
	}
	git("init")
	asset := filepath.Join(repo, "static/images/pod.svg")
	if err := os.MkdirAll(filepath.Dir(asset), 0700); err != nil {
		t.Fatal(err)
	}
	original := []byte("synthetic original bytes")
	if err := os.WriteFile(asset, original, 0600); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("commit", "-m", "fixture")
	commit := git("rev-parse", "HEAD")
	if err := os.WriteFile(asset, []byte("changed after commit"), 0600); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "index.html")
	write := func(src string) {
		t.Helper()
		if err := os.WriteFile(file, []byte(`<main class="td-content"><img src="`+src+`" alt="diagram"><img src="`+src+`" alt="another use"></main>`), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("/images/pod.svg")
	var out bytes.Buffer
	args := []string{"-file", file, "-repository", repo, "-commit", commit}
	if err := run(ctx, args, &out); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(original)
	for _, want := range []string{"image_references\t2", "image_capture\tlocal_only", "unique_assets\t1", "published\tfalse"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %q: %s", want, &out)
		}
	}
	// Compare captured bytes directly as well as the CLI's explicit nonpublication report.
	a := htmlarticle.Article{Blocks: []htmlarticle.Block{{Kind: htmlarticle.Image, SourceURL: "/images/pod.svg"}}}
	captured, err := captureImages(ctx, a, repo, commit)
	if err != nil || len(captured.Files) != 1 || captured.Files[0].SHA256 != digest {
		t.Fatal("wrong pinned asset", err)
	}
	for _, src := range []string{"/images/missing.svg", "https://example.invalid/new.svg"} {
		write(src)
		out.Reset()
		if err := run(ctx, args, &out); err == nil || out.Len() != 0 {
			t.Fatal("failed asset produced partial report", err, &out)
		}
	}
	out.Reset()
	if err := run(ctx, []string{"-file", file, "-repository", repo}, &out); err == nil || out.Len() != 0 {
		t.Fatal("unpaired identity accepted")
	}
}
