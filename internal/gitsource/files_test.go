package gitsource

import (
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func fileBudgets() FileLimits {
	return FileLimits{MaxFiles: 8, MaxListingBytes: 16 << 10, MaxFileBytes: 1024, MaxTotalBytes: 4096}
}

func TestCaptureFilesExactRevisionAndLiteralPaths(t *testing.T) {
	for _, format := range []string{"sha1", "sha256"} {
		t.Run(format, func(t *testing.T) {
			f := newFixture(t, format)
			original := "\x00\xff\x01binary figure"
			f.write(t, "static/images/a.bin", original)
			f.write(t, "static/images/:(glob)*.svg", "literal name")
			f.write(t, "static/images/sibling.svg", "unselected")
			old := f.commit(t)
			f.write(t, "static/images/a.bin", "new committed figure")
			newer := f.commit(t)
			f.write(t, "static/images/a.bin", "dirty figure")
			f.git(t, "replace", old, newer)
			paths := []string{"static/images/a.bin", "static/images/:(glob)*.svg"}
			req := FilesRequest{Repository: f.repo, Commit: old, Paths: paths}
			got, err := CaptureFiles(f.ctx, req, fileBudgets())
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(paths, req.Paths) || paths[0] != "static/images/a.bin" {
				t.Fatal("request mutated")
			}
			if got.Commit != old || len(got.Files) != 2 {
				t.Fatalf("wrong selection: %+v", got)
			}
			for i, want := range []struct{ name, body string }{
				{"static/images/:(glob)*.svg", "literal name"}, {"static/images/a.bin", original},
			} {
				file := got.Files[i]
				if file.Path != want.name || string(file.Body) != want.body || file.SHA256 != sha256.Sum256([]byte(want.body)) || !matchesBlob(file.Body, file.BlobID) {
					t.Fatalf("wrong bytes/identity for %q", want.name)
				}
			}
			got.Files[1].Body[0] = 42
			again, err := CaptureFiles(f.ctx, req, fileBudgets())
			if err != nil || string(again.Files[1].Body) != original {
				t.Fatal("result aliases a later capture", err)
			}
			empty, err := CaptureFiles(f.ctx, FilesRequest{Repository: f.repo, Commit: old}, fileBudgets())
			if err != nil || empty.Commit != old || len(empty.Files) != 0 {
				t.Fatal("empty selection failed", err)
			}
		})
	}
}

func TestCaptureFilesRejectsIncompleteOrUnsupportedSelection(t *testing.T) {
	f := newFixture(t, "sha1")
	f.write(t, "images/good", "valid")
	f.write(t, "images/pointer", "version https://git-lfs.github.com/spec/v1\n")
	if err := os.Symlink("good", filepath.Join(f.repo, "images/link")); err != nil {
		t.Fatal(err)
	}
	first := f.commit(t)
	f.git(t, "update-index", "--add", "--cacheinfo", "160000,"+first+",submodule")
	f.git(t, "commit", "-m", "gitlink")
	commit := f.git(t, "rev-parse", "HEAD")
	for _, tc := range []struct {
		name  string
		paths []string
		want  error
	}{
		{"missing", []string{"images/good", "images/absent"}, ErrMissing},
		{"tree", []string{"images"}, ErrUnsupported},
		{"symlink", []string{"images/good", "images/link"}, ErrUnsupported},
		{"submodule", []string{"submodule"}, ErrUnsupported},
		{"LFS after valid bytes", []string{"images/good", "images/pointer"}, ErrUnsupported},
		{"duplicate", []string{"images/good", "images/good"}, ErrInvalid},
		{"traversal", []string{"../images/good"}, ErrInvalid},
		{"unclean", []string{"images/./good"}, ErrInvalid},
		{"root", []string{"."}, ErrInvalid},
		{"absolute", []string{"/images/good"}, ErrInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := CaptureFiles(f.ctx, FilesRequest{Repository: f.repo, Commit: commit, Paths: tc.paths}, fileBudgets())
			if !errors.Is(err, tc.want) || !reflect.DeepEqual(got, FilesSnapshot{}) {
				t.Fatalf("partial or incorrect failure: %+v %v", got, err)
			}
		})
	}
}

func TestCaptureFilesBudgetsAndContext(t *testing.T) {
	f := newFixture(t, "sha1")
	f.write(t, "a", "abc")
	f.write(t, "b", "def")
	commit := f.commit(t)
	req := FilesRequest{Repository: f.repo, Commit: commit, Paths: []string{"a", "b"}}
	limits := FileLimits{MaxFiles: 2, MaxListingBytes: 4096, MaxFileBytes: 3, MaxTotalBytes: 6}
	if _, err := CaptureFiles(f.ctx, req, limits); err != nil {
		t.Fatal("inclusive size limits", err)
	}
	for _, tc := range []struct {
		name   string
		change func(*FileLimits)
		want   error
	}{
		{"file", func(l *FileLimits) { l.MaxFileBytes = 2 }, ErrLimit},
		{"total", func(l *FileLimits) { l.MaxTotalBytes = 5 }, ErrLimit},
		{"count", func(l *FileLimits) { l.MaxFiles = 1 }, ErrLimit},
		{"listing", func(l *FileLimits) { l.MaxListingBytes = 1 }, ErrLimit},
		{"invalid budget", func(l *FileLimits) { l.MaxFileBytes = 0 }, ErrInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			l := limits
			tc.change(&l)
			got, err := CaptureFiles(f.ctx, req, l)
			if !errors.Is(err, tc.want) || !reflect.DeepEqual(got, FilesSnapshot{}) {
				t.Fatalf("incorrect limit behavior: %+v %v", got, err)
			}
		})
	}
	if _, err := CaptureFiles(context.Background(), req, limits); !errors.Is(err, ErrInvalid) {
		t.Fatal("deadline required", err)
	}
	cancelled, cancel := context.WithCancel(f.ctx)
	cancel()
	if _, err := CaptureFiles(cancelled, req, limits); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation lost", err)
	}
	req.Commit = f.git(t, "rev-parse", commit+"^{tree}")
	if _, err := CaptureFiles(f.ctx, req, limits); !errors.Is(err, ErrInvalid) {
		t.Fatal("noncommit accepted", err)
	}
}
