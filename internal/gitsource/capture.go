// Package gitsource captures committed Markdown and explicit file dependencies
// from a trusted local Git repository. It does not fetch, process content, authorize access,
// fingerprint import requests or publish durable snapshots.
package gitsource

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"path"
	"strings"
)

var (
	ErrInvalid     = errors.New("invalid Git capture request")
	ErrLimit       = errors.New("Git capture limit exceeded")
	ErrUnsupported = errors.New("unsupported Git capture entry")
	ErrCorrupt     = errors.New("malformed Git object or metadata")
)

// Request identifies physical acquisition, not logical source identity or product
// version. Commit must be a full lowercase SHA-1 or SHA-256 commit ID. Directory
// is a clean repository-relative directory, or "." for the whole repository.
// The caller owns the trusted repository and must keep its objects available.
type Request struct {
	Repository string
	Commit     string
	Directory  string
}

// Limits bound application output/retained data, not every resource used inside
// Git. All fields must be positive; the context must also have a deadline.
type Limits struct {
	MaxEntries       int
	MaxListingBytes  int64
	MaxDocumentBytes int64
	MaxTotalBytes    int64
}

// Document contains raw committed bytes; no Markdown or text validity is implied.
// Path is repository-relative. SHA256 checks raw bytes; BlobID is Git's object ID.
// Neither is a logical document ID or authorization decision.
type Document struct {
	Path   string
	BlobID string
	Body   []byte
	SHA256 [sha256.Size]byte
}

// Snapshot and its document buffers belong to the caller. Ignored counts entries
// outside the explicit case-sensitive .md selection. Zero Documents describes an
// empty selection, not a decision to publish deletions. This is not a disk format.
type Snapshot struct {
	Commit    string
	Directory string
	Documents []Document
	Ignored   int
}

// Capture reads only committed objects and returns no partial snapshot on failure.
// The bounded in-memory baseline retains all selected bytes. Callers must authorize
// the source before access, bound concurrent captures, and arrange remote storage
// separately. Local objects may disappear; success here is not import publication.
func Capture(ctx context.Context, request Request, limits Limits) (Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	_, deadline := ctx.Deadline()
	maxInt := int64(int(^uint(0) >> 1))
	if !deadline || request.Repository == "" || strings.ContainsRune(request.Repository, 0) ||
		!objectID(request.Commit, 0) || !cleanPath(request.Directory, true) ||
		limits.MaxEntries <= 0 || limits.MaxListingBytes <= 0 || limits.MaxListingBytes > maxInt ||
		limits.MaxDocumentBytes <= 0 || limits.MaxTotalBytes < limits.MaxDocumentBytes || limits.MaxTotalBytes > maxInt {
		return Snapshot{}, ErrInvalid
	}
	treeID, err := commitTree(ctx, request.Repository, request.Commit)
	if err != nil {
		return Snapshot{}, err
	}
	if request.Directory != "." {
		listing, err := run(ctx, request.Repository, limits.MaxListingBytes, "ls-tree", "--full-tree", "-z", "-l", treeID, "--", request.Directory)
		if err != nil {
			return Snapshot{}, fmt.Errorf("locate capture directory: %w", err)
		}
		entries, err := parseListing(listing, 1, len(request.Commit))
		if err != nil {
			return Snapshot{}, err
		}
		if len(entries) != 1 || entries[0].name != request.Directory || entries[0].mode != "040000" {
			return Snapshot{}, ErrInvalid
		}
		treeID = entries[0].id
	}
	listing, err := run(ctx, request.Repository, limits.MaxListingBytes, "ls-tree", "--full-tree", "-r", "-z", "-l", treeID)
	if err != nil {
		return Snapshot{}, fmt.Errorf("list capture entries: %w", err)
	}
	entries, err := parseListing(listing, limits.MaxEntries, len(request.Commit))
	if err != nil {
		return Snapshot{}, err
	}
	result := Snapshot{Commit: request.Commit, Directory: request.Directory}
	var total int64
	for _, entry := range entries {
		if entry.mode == "160000" || entry.mode == "040000" {
			return Snapshot{}, ErrUnsupported
		}
		if !strings.HasSuffix(entry.name, ".md") {
			result.Ignored++
			continue
		}
		if entry.mode == "120000" {
			return Snapshot{}, ErrUnsupported
		}
		if entry.size > limits.MaxDocumentBytes || entry.size > limits.MaxTotalBytes-total {
			return Snapshot{}, ErrLimit
		}
		total += entry.size
		name := entry.name
		if request.Directory != "." {
			name = path.Join(request.Directory, name)
		}
		if !cleanPath(name, false) {
			return Snapshot{}, ErrUnsupported
		}
		result.Documents = append(result.Documents, Document{Path: name, BlobID: entry.id})
	}
	// Check every advertised size before reading any selected blob.
	selected := 0
	for _, entry := range entries {
		if !strings.HasSuffix(entry.name, ".md") {
			continue
		}
		file, err := captureBlob(ctx, request.Repository, entry, result.Documents[selected].Path)
		if err != nil {
			return Snapshot{}, err
		}
		result.Documents[selected] = file
		selected++
	}
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	return result, nil
}
