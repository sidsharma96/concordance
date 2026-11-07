package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sidsharma96/concordance/tools/internal/textnav"
)

func corpusFixture(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	body := []byte("first\nStrong comparison: é\nlast\n")
	if err := os.WriteFile(filepath.Join(dir, "rfc1.txt"), body, 0600); err != nil {
		t.Fatal(err)
	}
	pins := filepath.Join(dir, "selected.pins")
	if err := os.WriteFile(pins, fmt.Appendf(nil, "rfc1 %x\n", sha256.Sum256(body)), 0600); err != nil {
		t.Fatal(err)
	}
	return dir, pins
}

func TestNavigationCommands(t *testing.T) {
	dir, pins := corpusFixture(t)
	for _, tc := range []struct {
		name string
		args []string
		want []string
	}{
		{"lookup", nil, []string{"corpus documents=1", "document id=rfc1", "session calls=1 returned-source-bytes=0"}},
		{"find", []string{"-op", "find", "-literal", "Strong comparison"}, []string{"match [6,23)", "more=false", "session calls=2 returned-source-bytes=0"}},
		{"read", []string{"-op", "read", "-start", "6", "-end", "23"}, []string{`excerpt [6,23) text="Strong comparison"`, "session calls=2 returned-source-bytes=17"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			args := append([]string{"-root", dir, "-pins", pins, "-id", "rfc1"}, tc.args...)
			if err := run(context.Background(), args, &out); err != nil {
				t.Fatal(err)
			}
			for _, want := range tc.want {
				if !strings.Contains(out.String(), want) {
					t.Fatalf("missing %q: %s", want, out.String())
				}
			}
		})
	}
}

func TestNavigationFailsWithoutSuccessOutput(t *testing.T) {
	dir, pins := corpusFixture(t)
	for _, more := range [][]string{
		{"-op", "find", "-literal", ""},
		{"-op", "find", "-literal", "x", "-from", "-1"},
		{"-op", "find", "-literal", "x", "-limit", "17"},
		{"-op", "read", "-start", "0", "-end", "1000"},
		{"-id", "rfc2"},
		{"-exclude", "rfc2"},
		{"-op", "unknown"},
		{"-timeout", "0"},
	} {
		var out bytes.Buffer
		args := append([]string{"-root", dir, "-pins", pins, "-id", "rfc1"}, more...)
		if err := run(context.Background(), args, &out); err == nil || out.Len() != 0 {
			t.Fatalf("partial success for %v: %q %v", more, out.String(), err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "rfc1.txt"), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run(context.Background(), []string{"-root", dir, "-pins", pins, "-id", "rfc1"}, &out); !errors.Is(err, textnav.ErrBinding) || out.Len() != 0 {
		t.Fatalf("changed source: %q %v", out.String(), err)
	}
}

type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) { return len(p) - 1, nil }

func TestOutputWriteChecked(t *testing.T) {
	dir, pins := corpusFixture(t)
	if err := run(context.Background(), []string{"-root", dir, "-pins", pins, "-id", "rfc1"}, shortWriter{}); !errors.Is(err, io.ErrShortWrite) {
		t.Fatal(err)
	}
}

func TestPinsAndExplicitExclusions(t *testing.T) {
	h := fmt.Sprintf("%x", sha256.Sum256([]byte("body")))
	body := []byte("rfc1 " + h + "\nrfc2 " + h + "\n")
	pins, err := parsePins(body, "rfc2")
	if err != nil || len(pins) != 1 || pins[0].ID != "rfc1" || pins[0].Path != "rfc1.txt" {
		t.Fatal(pins, err)
	}
	for _, tc := range []struct{ body, excluded string }{
		{"", ""},
		{"rfc01 " + h, ""},
		{"rfc+1 " + h, ""},
		{"rfc1  " + h, ""},
		{"rfc1 " + h + "\nrfc1 " + h, ""},
		{"rfc1 invalid\nrfc2 " + h, "rfc1"},
		{string(body), "rfc3"},
		{string(body), "rfc2,rfc2"},
		{string(body), "rfc1,rfc2"},
		{"rfc1 " + strings.Repeat("x", 300), ""},
	} {
		if got, err := parsePins([]byte(tc.body), tc.excluded); err == nil || got != nil {
			t.Fatalf("invalid inventory/exclusion: %+v %v", got, err)
		}
	}
	many := new(strings.Builder)
	for i := 1; i <= 1001; i++ {
		fmt.Fprintf(many, "rfc%d %s\n", i, h)
	}
	if _, err := parsePins([]byte(many.String()), ""); !errors.Is(err, textnav.ErrLimit) {
		t.Fatalf("count ceiling: %v", err)
	}
}
