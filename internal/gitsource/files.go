package gitsource

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// FilesRequest selects exact committed paths, independent of content format.
// Repository and Commit follow Request's trust and identity requirements.
// Paths must be unique, clean repository-relative paths; order is immaterial.
type FilesRequest struct {
	Repository string
	Commit     string
	Paths      []string
}

// FileLimits bound selected metadata and owned bytes, not Git's internal work.
// All fields must be positive; the context must also have a deadline.
type FileLimits struct {
	MaxFiles        int
	MaxListingBytes int64
	MaxFileBytes    int64
	MaxTotalBytes   int64
}

// FilesSnapshot owns its raw file buffers. Document here carries path/blob/byte
// metadata only; a selected file need not be Markdown or a logical document.
// This is neither a persisted representation nor a published generation.
type FilesSnapshot struct {
	Commit string
	Files  []Document
}

// ErrMissing means at least one requested committed path was not found.
var ErrMissing = errors.New("requested Git file missing")

// CaptureFiles captures explicit regular-file dependencies at an exact revision.
// On failure it returns no partial result. Callers must authorize access and bound
// concurrent calls and generation-wide work. No network fetch or remote retention
// occurs. An empty selection is valid after validating the commit.
func CaptureFiles(ctx context.Context, request FilesRequest, limits FileLimits) (FilesSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return FilesSnapshot{}, err
	}
	_, deadline := ctx.Deadline()
	maxInt := int64(int(^uint(0) >> 1))
	if !deadline || request.Repository == "" || strings.ContainsRune(request.Repository, 0) ||
		!objectID(request.Commit, 0) || limits.MaxFiles <= 0 ||
		limits.MaxListingBytes <= 0 || limits.MaxListingBytes > maxInt || limits.MaxFileBytes <= 0 ||
		limits.MaxTotalBytes < limits.MaxFileBytes || limits.MaxTotalBytes > maxInt {
		return FilesSnapshot{}, ErrInvalid
	}
	if len(request.Paths) > limits.MaxFiles {
		return FilesSnapshot{}, ErrLimit
	}
	for _, name := range request.Paths {
		if !cleanPath(name, false) {
			return FilesSnapshot{}, ErrInvalid
		}
	}
	paths := slices.Clone(request.Paths)
	slices.Sort(paths)
	for i := 1; i < len(paths); i++ {
		if paths[i-1] == paths[i] {
			return FilesSnapshot{}, ErrInvalid
		}
	}
	treeID, err := commitTree(ctx, request.Repository, request.Commit)
	if err != nil {
		return FilesSnapshot{}, err
	}
	if len(paths) == 0 {
		return FilesSnapshot{Commit: request.Commit}, nil
	}
	args := []string{"ls-tree", "--full-tree", "-z", "-l", treeID, "--"}
	listing, err := run(ctx, request.Repository, limits.MaxListingBytes, append(args, paths...)...)
	if err != nil {
		return FilesSnapshot{}, fmt.Errorf("list requested Git files: %w", err)
	}
	entries, err := parseListing(listing, limits.MaxFiles, len(request.Commit))
	if err != nil {
		return FilesSnapshot{}, err
	}
	if len(entries) != len(paths) {
		return FilesSnapshot{}, ErrMissing
	}
	var total int64
	for i, entry := range entries {
		if entry.name != paths[i] {
			return FilesSnapshot{}, ErrMissing
		}
		if entry.mode != "100644" && entry.mode != "100755" {
			return FilesSnapshot{}, ErrUnsupported
		}
		if entry.size > limits.MaxFileBytes || entry.size > limits.MaxTotalBytes-total {
			return FilesSnapshot{}, ErrLimit
		}
		total += entry.size
	}
	result := FilesSnapshot{Commit: request.Commit, Files: make([]Document, 0, len(entries))}
	for _, entry := range entries {
		file, err := captureBlob(ctx, request.Repository, entry, entry.name)
		if err != nil {
			return FilesSnapshot{}, err
		}
		result.Files = append(result.Files, file)
	}
	if err := ctx.Err(); err != nil {
		return FilesSnapshot{}, err
	}
	return result, nil
}
