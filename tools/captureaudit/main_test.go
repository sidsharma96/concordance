package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sidsharma96/concordance/internal/capturefiles"
	"github.com/sidsharma96/concordance/internal/htmlarticle"
)

func fixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	recipe, html := []byte("authored fixture"), []byte(`<main class="fixture"><p>Pods</p></main>`)
	ref := func(p string, b []byte) capturefiles.File {
		return capturefiles.File{Path: p, SHA256: sha256.Sum256(b), Bytes: int64(len(b))}
	}
	d := capturefiles.Description{Editions: []capturefiles.Edition{{ID: "one", Captures: []capturefiles.Capture{{Source: "docs", Revision: strings.Repeat("a", 40), Directory: "docs", Preparation: "fixture/v1", Toolchain: "authored", Profile: htmlarticle.Profile{ID: "fixture/v1", ArticleClass: "fixture"}, Recipe: ref("recipe", recipe), Pages: []capturefiles.Page{{Path: "docs/a.md", HTML: ref("page", html)}}}}}}}
	b, err := capturefiles.Encode(ctx, d, capturefiles.Limits{MaxDescriptorBytes: 4096, MaxEditions: 1, MaxCaptures: 1, MaxPages: 1, MaxDependencies: 1, MaxFiles: 2, MaxStringBytes: 256, MaxValueBytes: 4096, MaxFileBytes: 4096, MaxPayloadBytes: 8192})
	if err != nil {
		t.Fatal(err)
	}
	for p, data := range map[string][]byte{"recipe": recipe, "page": html, capturefiles.DescriptorName: b} {
		if err := os.WriteFile(filepath.Join(root, p), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}
func TestCaptureAuditOnlyReportsCompleteFileChecks(t *testing.T) {
	root := fixture(t)
	var out bytes.Buffer
	if err := run(t.Context(), []string{"-directory", root}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "pages\t1\n") || !strings.Contains(out.String(), "file_checks\tPASS\n") {
		t.Fatal(out.String())
	}
	if err := os.WriteFile(filepath.Join(root, "page"), []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := run(t.Context(), []string{"-directory", root}, &out); err == nil || out.Len() != 0 {
		t.Fatal("partial audit report", out.String(), err)
	}
}

type failedWriter struct{}

func (failedWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }
func TestCaptureAuditBoundaries(t *testing.T) {
	root := fixture(t)
	for _, args := range [][]string{{}, {"-directory", root, "extra"}, {"-directory", root, "-timeout", "0s"}, {"-directory", root, "-max-file-bytes", "1"}, {"-bad"}} {
		var out bytes.Buffer
		if err := run(t.Context(), args, &out); err == nil || out.Len() != 0 {
			t.Fatal(args, out.String(), err)
		}
	}
	if err := run(t.Context(), []string{"-directory", root}, failedWriter{}); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal(err)
	}
}
