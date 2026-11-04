package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/sidsharma96/concordance/internal/importjob"
	"github.com/sidsharma96/concordance/internal/jsonunicode"
	"github.com/sidsharma96/concordance/internal/preparedimport"
)

const sourcesFormat = "concordance.git-sources/v1"
const maxSourcesBytes = 1 << 20
const maxImportSources = 32
const maxSourceString = 8192

var errSources = errors.New("invalid trusted source profile")

// Profiles are trusted operator registration, never taken from a capture/job.
// A profile names exactly this job's sources; paths are absolute local routes.
// The native root/regular checks assume stable protected local paths, not hostile
// replacement. Context cannot forcibly interrupt blocked filesystem operations.
func readSources(ctx context.Context, path string) (out []preparedimport.Source, err error) {
	if path == "" || len(path) > maxSourceString {
		return nil, errSources
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	defer func() {
		err = errors.Join(err, root.Close())
		if err != nil {
			out = nil
		}
	}()
	name := filepath.Base(path)
	info, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxSourcesBytes {
		return nil, errSources
	}
	f, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer func() {
		err = errors.Join(err, f.Close())
		if err != nil {
			out = nil
		}
	}()
	info, err = f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxSourcesBytes {
		return nil, errSources
	}
	body, err := io.ReadAll(io.LimitReader(f, maxSourcesBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxSourcesBytes {
		return nil, errSources
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return decodeSources(ctx, body)
}

func sourceText(s string) bool {
	return s != "" && len(s) <= maxSourceString && utf8.ValidString(s) && strings.IndexFunc(s, unicode.IsControl) < 0
}

func decodeSources(ctx context.Context, body []byte) ([]preparedimport.Source, error) {
	if len(body) > maxSourcesBytes || !utf8.Valid(body) || jsonunicode.CheckSurrogates(ctx, body) != nil {
		return nil, errSources
	}
	d := json.NewDecoder(bytes.NewReader(body))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return nil, errSources
	}
	seen := map[string]bool{}
	ids := map[string]bool{}
	out := []preparedimport.Source{}
	for d.More() {
		t, err := d.Token()
		name, ok := t.(string)
		if err != nil || !ok || seen[name] {
			return nil, errSources
		}
		seen[name] = true
		switch name {
		case "format":
			t, err := d.Token()
			if err != nil || t != sourcesFormat {
				return nil, errSources
			}
		case "sources":
			t, err := d.Token()
			if err != nil || t != json.Delim('[') {
				return nil, errSources
			}
			for d.More() {
				if len(out) >= maxImportSources {
					return nil, errSources
				}
				s, err := decodeSource(d)
				if err != nil || ids[s.ID] {
					return nil, errSources
				}
				ids[s.ID] = true
				out = append(out, s)
			}
			t, err = d.Token()
			if err != nil || t != json.Delim(']') {
				return nil, errSources
			}
		default:
			return nil, errSources
		}
	}
	token, err = d.Token()
	if err != nil || token != json.Delim('}') || len(seen) != 2 {
		return nil, errSources
	}
	if _, err := d.Token(); !errors.Is(err, io.EOF) {
		return nil, errSources
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func decodeSource(d *json.Decoder) (preparedimport.Source, error) {
	t, err := d.Token()
	if err != nil || t != json.Delim('{') {
		return preparedimport.Source{}, errSources
	}
	values := map[string]string{}
	for d.More() {
		t, err := d.Token()
		name, ok := t.(string)
		if err != nil || !ok || values[name] != "" || (name != "id" && name != "repository" && name != "path") {
			return preparedimport.Source{}, errSources
		}
		t, err = d.Token()
		value, ok := t.(string)
		if err != nil || !ok || !sourceText(value) {
			return preparedimport.Source{}, errSources
		}
		values[name] = value
	}
	t, err = d.Token()
	if err != nil || t != json.Delim('}') || len(values) != 3 || len(values["id"]) > 256 || !filepath.IsAbs(values["path"]) {
		return preparedimport.Source{}, errSources
	}
	return preparedimport.Source{ID: values["id"], Repository: values["repository"], LocalPath: values["path"]}, nil
}

func jobRequest(collection string, sources []preparedimport.Source) importjob.Request {
	r := importjob.Request{Collection: collection, Sources: make([]importjob.Source, len(sources))}
	for i, s := range sources {
		r.Sources[i] = importjob.Source{ID: s.ID, Repository: s.Repository}
	}
	return r
}

func matchJob(saved importjob.Saved, collection string, sources []preparedimport.Source) bool {
	if saved.Collection != collection || len(saved.Sources) != len(sources) {
		return false
	}
	expected := make(map[string]string, len(sources))
	for _, s := range sources {
		expected[s.ID] = s.Repository
	}
	for _, s := range saved.Sources {
		if expected[s.ID] != s.Repository {
			return false
		}
		delete(expected, s.ID)
	}
	return len(expected) == 0
}
