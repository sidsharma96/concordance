package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sidsharma96/concordance/internal/htmlpassage"
)

func TestAuditPassagesAndNoPartialReport(t *testing.T) {
	file := filepath.Join(t.TempDir(), "article.html")
	body := `<main class="td-content"><h1>Private fixture title</h1><div class="feature-state-notice" title="Feature Gate: Example"><p>Beta; enabled.</p></div><p>Private explanation.</p><pre><code>  value: true
</code></pre></main>`
	if err := os.WriteFile(file, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run(context.Background(), []string{"-file", file, "-passages"}, &out); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"passage_processor\thtmlpassage/v1\n", "passages\t3\n", "published\tfalse\n"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %q: %s", want, &out)
		}
	}
	if strings.Contains(out.String(), "Private") || strings.Contains(out.String(), "Feature Gate") {
		t.Fatal("body or metadata label leaked")
	}
	out.Reset()
	err := run(context.Background(), []string{"-file", file, "-passages", "-max-passage-bytes", "1"}, &out)
	if !errors.Is(err, htmlpassage.ErrLimit) || out.Len() != 0 {
		t.Fatal("partial report or lost limit error", err, &out)
	}
	out.Reset()
	if err := run(context.Background(), []string{"-file", file, "-max-passage-bytes", "8192"}, &out); err == nil || out.Len() != 0 {
		t.Fatal("orphan passage limit accepted")
	}
}
