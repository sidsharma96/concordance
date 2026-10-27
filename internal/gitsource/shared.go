package gitsource

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
)

func commitTree(ctx context.Context, repository, commit string) (string, error) {
	typeName, err := run(ctx, repository, 16, "cat-file", "-t", commit)
	if err != nil {
		return "", fmt.Errorf("inspect capture commit: %w", err)
	}
	if string(typeName) != "commit\n" {
		return "", ErrInvalid
	}
	tree, err := run(ctx, repository, 80, "rev-parse", "--verify", commit+"^{tree}")
	if err != nil {
		return "", fmt.Errorf("resolve capture tree: %w", err)
	}
	treeID := strings.TrimSuffix(string(tree), "\n")
	if !objectID(treeID, len(commit)) {
		return "", ErrCorrupt
	}
	return treeID, nil
}

func captureBlob(ctx context.Context, repository string, entry treeEntry, name string) (Document, error) {
	body, err := run(ctx, repository, entry.size, "cat-file", "blob", entry.id)
	if err != nil {
		return Document{}, fmt.Errorf("read captured blob: %w", err)
	}
	if int64(len(body)) != entry.size || !matchesBlob(body, entry.id) {
		return Document{}, ErrCorrupt
	}
	if bytes.HasPrefix(body, []byte("version https://git-lfs.github.com/spec/")) {
		return Document{}, ErrUnsupported
	}
	return Document{Path: name, BlobID: entry.id, Body: body, SHA256: sha256.Sum256(body)}, nil
}
