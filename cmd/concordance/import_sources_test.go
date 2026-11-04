package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func profileBytes(path string) []byte {
	body, _ := json.Marshal(map[string]any{"format": sourcesFormat, "sources": []any{map[string]string{"id": "docs", "repository": "https://example.invalid/docs.git", "path": path}}})
	return body
}

func TestSourcesProfileStrictAdmission(t *testing.T) {
	ctx := testContext(t)
	good := string(profileBytes(t.TempDir()))
	if got, err := decodeSources(ctx, []byte(good)); err != nil || len(got) != 1 || got[0].ID != "docs" {
		t.Fatal(got, err)
	}
	source := "{\"id\":\"docs\",\"repository\":\"repo\",\"path\":\"/tmp/repo\"}"
	bad := []string{
		"", "null", "[]", "{}", good + " {}", strings.Replace(good, "\"format\"", "\"FORMAT\"", 1),
		strings.Replace(good, "\"sources\"", "\"source\"", 1), strings.Replace(good, sourcesFormat, "next", 1),
		strings.Replace(good, "\"id\"", "\"ID\"", 1), strings.Replace(good, "\"id\":", "\"id\":\"other\",\"id\":", 1),
		strings.Replace(good, "\"repository\":", "\"secret\":\"password\",\"repository\":", 1),
		"{\"format\":\"" + sourcesFormat + "\",\"format\":\"" + sourcesFormat + "\",\"sources\":[]}",
		"{\"format\":\"" + sourcesFormat + "\",\"sources\":null}",
		"{\"format\":\"" + sourcesFormat + "\",\"sources\":[" + source + "," + source + "]}",
		"{\"format\":\"" + sourcesFormat + "\",\"sources\":[{\"id\":\"docs\",\"repository\":\"repo\"}]}",
		"{\"format\":\"" + sourcesFormat + "\",\"sources\":[{\"id\":null,\"repository\":\"repo\",\"path\":\"/tmp/repo\"}]}",
		strings.Replace(good, "docs.git", "bad\\ud800", 1),
		string([]byte{0xff}),
		strings.Replace(good, "\"docs\"", "\"bad\\nname\"", 1),
		strings.Replace(good, "\"docs\"", "\""+strings.Repeat("x", 257)+"\"", 1),
	}
	for i, body := range bad {
		if got, err := decodeSources(ctx, []byte(body)); err == nil || got != nil {
			t.Fatalf("case%d accepted: %v", i, err)
		}
	}
	tooMany := make([]string, maxImportSources+1)
	for i := range tooMany {
		b, _ := json.Marshal(map[string]string{"id": strings.Repeat("x", i+1), "repository": "repo", "path": "/tmp/repo"})
		tooMany[i] = string(b)
	}
	for _, body := range []string{
		"{\"format\":\"" + sourcesFormat + "\",\"sources\":[" + strings.Join(tooMany, ",") + "]}",
		strings.Repeat(" ", maxSourcesBytes+1),
		"{\"format\":\"" + sourcesFormat + "\",\"sources\":[{\"id\":\"docs\",\"repository\":\"repo\",\"path\":\"relative\"}]}",
	} {
		if got, err := decodeSources(ctx, []byte(body)); err == nil || got != nil {
			t.Fatal("accepted excess/relative profile")
		}
	}
	directory := t.TempDir()
	path := filepath.Join(directory, "sources")
	if err := os.WriteFile(path, []byte(good), 0600); err != nil {
		t.Fatal(err)
	}
	if got, err := readSources(ctx, path); err != nil || len(got) != 1 {
		t.Fatal(got, err)
	}
	if err := os.Symlink(path, filepath.Join(directory, "link")); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{directory, filepath.Join(directory, "link")} {
		if got, err := readSources(ctx, path); err == nil || got != nil {
			t.Fatal("accepted nonregular profile")
		}
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if got, err := readSources(canceled, path); !errors.Is(err, context.Canceled) || got != nil {
		t.Fatal(got, err)
	}
}

func FuzzSourcesProfile(f *testing.F) {
	f.Add(profileBytes("/tmp/repo"))
	f.Add([]byte("{}"))
	f.Fuzz(func(t *testing.T, body []byte) {
		if len(body) > maxSourcesBytes {
			return
		}
		got, err := decodeSources(context.Background(), body)
		if err != nil {
			if got != nil {
				t.Fatal("partial sources")
			}
			return
		}
		if len(got) > maxImportSources {
			t.Fatal("source count")
		}
		seen := map[string]bool{}
		for _, s := range got {
			if seen[s.ID] || !sourceText(s.ID) || !sourceText(s.Repository) || !filepath.IsAbs(s.LocalPath) {
				t.Fatal("invalid accepted sources")
			}
			seen[s.ID] = true
		}
	})
}
