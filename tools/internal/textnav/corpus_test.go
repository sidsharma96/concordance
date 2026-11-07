package textnav

import (
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func openLimits() OpenLimits {
	return OpenLimits{MaxDocuments: 8, MaxDocumentBytes: 4096, MaxCorpusBytes: 8192, MaxMetadataBytes: 1024}
}

func fixture(t *testing.T, bodies ...string) (*os.Root, []Pin, string) {
	t.Helper()
	dir := t.TempDir()
	pins := make([]Pin, len(bodies))
	for i, body := range bodies {
		id := string(rune('a' + i))
		pins[i] = Pin{ID: id, Path: id + ".txt", SHA256: sha256.Sum256([]byte(body))}
		if err := os.WriteFile(filepath.Join(dir, pins[i].Path), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	r, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close() })
	return r, pins, dir
}

func TestOpenOwnsVerifiedSnapshot(t *testing.T) {
	ctx := testContext(t)
	root, pins, dir := fixture(t, "original é🙂", "original é🙂", "")
	originalPin := pins[0]
	c, err := Open(ctx, root, pins, openLimits())
	if err != nil {
		t.Fatal(err)
	}
	if got := c.Inventory(); got.Documents != 3 || got.Bytes != 2*len("original é🙂") {
		t.Fatalf("inventory: %+v", got)
	}
	pins[0] = Pin{ID: "mutated"}
	if err := root.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("replaced"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "b.txt")); err != nil {
		t.Fatal(err)
	}
	s, err := c.NewSession(sessionLimits())
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"a", "b"} {
		d, err := s.Lookup(ctx, id)
		if err != nil || d.Ref.ID != id || d.Ref.SHA256 != originalPin.SHA256 {
			t.Fatalf("separate identity/shared bytes: %+v %v", d, err)
		}
		part, err := s.Read(ctx, d.Ref, Span{0, d.Bytes})
		if err != nil || part.Text != "original é🙂" {
			t.Fatalf("retained bytes after root close/file replacement: %+v %v", part, err)
		}
	}
	fresh, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()
	if got, err := Open(ctx, fresh, []Pin{originalPin}, openLimits()); got != nil || !errors.Is(err, ErrBinding) {
		t.Fatalf("new open accepted replacement: %v %v", got, err)
	}
}

func TestOpenAdmissionAndIntegrity(t *testing.T) {
	ctx := testContext(t)
	for _, tc := range []struct {
		name   string
		body   string
		mutate func([]Pin, *OpenLimits) []Pin
		want   error
	}{
		{"hash", "body", func(p []Pin, _ *OpenLimits) []Pin { p[0].SHA256[0] ^= 1; return p }, ErrBinding},
		{"utf8", "\xff", nil, ErrContent},
		{"nul", "x\x00y", nil, ErrContent},
		{"traversal", "body", func(p []Pin, _ *OpenLimits) []Pin { p[0].Path = "../a.txt"; return p }, ErrInvalid},
		{"id", "body", func(p []Pin, _ *OpenLimits) []Pin { p[0].ID = ""; return p }, ErrInvalid},
		{"metadata", "body", func(p []Pin, l *OpenLimits) []Pin { l.MaxMetadataBytes = 1; return p }, ErrLimit},
		{"file bytes", "body", func(p []Pin, l *OpenLimits) []Pin { l.MaxDocumentBytes = 3; return p }, ErrLimit},
		{"total bytes", "body", func(p []Pin, l *OpenLimits) []Pin { l.MaxCorpusBytes = 3; return p }, ErrLimit},
		{"bad limit", "body", func(p []Pin, l *OpenLimits) []Pin { l.MaxCorpusBytes = 0; return p }, ErrInvalid},
		{"count", "body", func(p []Pin, l *OpenLimits) []Pin { l.MaxDocuments = 1; return append(p, Pin{ID: "b", Path: "b.txt"}) }, ErrLimit},
		// The nonexistent first path proves metadata admission precedes ALL I/O.
		{"duplicate before IO", "body", func(p []Pin, _ *OpenLimits) []Pin { p[0].Path = "absent"; return append(p, p[0]) }, ErrInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, pins, _ := fixture(t, tc.body)
			l := openLimits()
			if tc.mutate != nil {
				pins = tc.mutate(pins, &l)
			}
			if c, err := Open(ctx, r, pins, l); c != nil || !errors.Is(err, tc.want) {
				t.Fatalf("partial corpus or wrong error: %v %v", c, err)
			}
		})
	}
	r, pins, dir := fixture(t, "body")
	if err := os.Symlink("a.txt", filepath.Join(dir, "link.txt")); err != nil {
		t.Fatal(err)
	}
	pins[0].Path = "link.txt"
	if c, err := Open(ctx, r, pins, openLimits()); c != nil || !errors.Is(err, ErrInvalid) {
		t.Fatalf("link: %v %v", c, err)
	}
	if c, err := Open(context.Background(), r, nil, openLimits()); c != nil || !errors.Is(err, ErrInvalid) {
		t.Fatalf("deadline: %v %v", c, err)
	}
	if c, err := Open(nil, r, nil, openLimits()); c != nil || !errors.Is(err, ErrInvalid) {
		t.Fatalf("nil context: %v %v", c, err)
	}
	if c, err := Open(ctx, r, nil, openLimits()); err != nil || c.Inventory().Documents != 0 {
		t.Fatalf("explicit empty selection: %v %v", c, err)
	}
}

func TestReadBodyDoesNotTrustEarlierSize(t *testing.T) {
	ctx := testContext(t)
	// This reader represents a stream that grew past an already admitted size.
	if body, err := readBody(ctx, strings.NewReader("grown"), 4); body != nil || !errors.Is(err, ErrLimit) {
		t.Fatalf("limit+1 read: %q %v", body, err)
	}
	if body, err := readBody(ctx, strings.NewReader("fits"), 4); err != nil || string(body) != "fits" {
		t.Fatalf("exact fit: %q %v", body, err)
	}
	failure := errors.New("reader failed")
	if body, err := readBody(ctx, io.MultiReader(strings.NewReader("ab"), errorReader{failure}), 10); body != nil || !errors.Is(err, failure) {
		t.Fatalf("partial read leaked: %q %v", body, err)
	}
}

type errorReader struct{ err error }

func (r errorReader) Read([]byte) (int, error) { return 0, r.err }
