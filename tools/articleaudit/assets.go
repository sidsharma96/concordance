package main

import (
	"context"
	"errors"
	"path"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/sidsharma96/concordance/internal/gitsource"
	"github.com/sidsharma96/concordance/internal/htmlarticle"
)

// This reference-only Kubernetes mapping deliberately supports root-relative
// /images/... assets. It neither fetches URLs nor guesses Hugo page-bundle paths.
// Reject alternate URL spellings instead of decoding/cleaning them into access.
func imagePath(address string) (string, error) {
	if len(address) > 1024-len("static") || !utf8.ValidString(address) ||
		!strings.HasPrefix(address, "/images/") || address == "/images/" ||
		strings.ContainsAny(address, "\\%?#:") || strings.IndexFunc(address, unicode.IsControl) >= 0 ||
		path.Clean(address) != address {
		return "", errors.New("unsupported Kubernetes reference image address")
	}
	return "static" + address, nil
}

// Count occurrences before deduplicating source paths. Each Image block retains
// its own association; this set only avoids repeated capture of the same path.
func imagePaths(article htmlarticle.Article) ([]string, error) {
	paths := make(map[string]struct{})
	references := 0
	for _, block := range article.Blocks {
		if block.Kind != htmlarticle.Image {
			continue
		}
		references++
		if references > 256 {
			return nil, gitsource.ErrLimit
		}
		name, err := imagePath(block.SourceURL)
		if err != nil {
			return nil, err
		}
		paths[name] = struct{}{}
		if len(paths) > 128 {
			return nil, gitsource.ErrLimit
		}
	}
	result := make([]string, 0, len(paths))
	for name := range paths {
		result = append(result, name)
	}
	slices.Sort(result)
	return result, nil
}

func captureImages(ctx context.Context, article htmlarticle.Article, repository, commit string) (gitsource.FilesSnapshot, error) {
	paths, err := imagePaths(article)
	if err != nil {
		return gitsource.FilesSnapshot{}, err
	}
	return gitsource.CaptureFiles(ctx, gitsource.FilesRequest{Repository: repository, Commit: commit, Paths: paths},
		gitsource.FileLimits{MaxFiles: 128, MaxListingBytes: 256 << 10, MaxFileBytes: 4 << 20, MaxTotalBytes: 16 << 20})
}
