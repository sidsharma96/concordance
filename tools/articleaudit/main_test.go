package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAuditBoundaryAndCoverage(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "index.html")
	content := `<nav>outside</nav><div class="td-content"><h1>private fixture title</h1><p>private fixture body</p><img alt="diagram" src="/figure.svg"><div id="pre-footer"><button>Yes</button></div></div>`
	if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run(context.Background(), []string{"-file", file}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "image_references\t1\nimage_content_interpreted\tfalse") || strings.Contains(out.String(), "private fixture") {
		t.Fatalf("coverage missing or body leaked: %s", &out)
	}
	link := filepath.Join(dir, "link.html")
	if err := os.Symlink(file, link); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"-file", link}, {"-file", dir}, {"-file", file, "-max-input-bytes", "10"}, {"-file", filepath.Join(dir, "_print", "index.html")}} {
		out.Reset()
		if err := run(context.Background(), args, &out); err == nil || out.Len() != 0 {
			t.Fatalf("invalid input produced a successful report: %v %s", err, &out)
		}
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	out.Reset()
	if err := run(cancelled, []string{"-file", file}, &out); !errors.Is(err, context.Canceled) || out.Len() != 0 {
		t.Fatalf("cancellation ignored: %v", err)
	}
}
