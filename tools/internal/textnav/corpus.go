// Package textnav opens a bounded verified UTF-8 corpus for offline experiments.
// It is not a production source adapter, persisted citation schema or permission
// boundary. Callers authorize and pin the complete selection before Open.
package textnav

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"
	"unicode/utf8"
)

var (
	ErrInvalid = errors.New("invalid text navigation input")
	ErrLimit   = errors.New("text navigation limit exceeded")
	ErrMissing = errors.New("text document not found")
	ErrBinding = errors.New("text document pin mismatch")
	ErrContent = errors.New("unsupported captured text")
)

// Pin separates logical identity, local location and exact captured bytes.
// Root-relative Path is trusted operator input, never an agent-supplied path.
// The caller keeps pins unchanged through Open; no input slice is retained.
type Pin struct {
	ID     string
	Path   string
	SHA256 [32]byte
}

// OpenLimits bound logical metadata and text, not whole-process heap. ReadAll and
// the independent string copy require scratch space in addition to retained text.
type OpenLimits struct {
	MaxDocuments     int
	MaxDocumentBytes int
	MaxCorpusBytes   int
	MaxMetadataBytes int
}

// Ref is a logical ID bound to exact bytes, not a temporary pointer or authority.
// A ref is accepted only if that ID/digest belongs to this opened corpus.
type Ref struct {
	ID     string
	SHA256 [32]byte
}

type Document struct {
	Ref   Ref
	Bytes int
}

type document struct {
	ref  Ref
	text string
}

// Corpus owns immutable text. It retains no root, file handles or context and may
// be shared by independently owned sessions. It must be obtained through Open.
type Corpus struct {
	documents map[string]document
	bytes     int
}

type Inventory struct {
	Documents int
	Bytes     int
}

func (c *Corpus) Inventory() Inventory {
	if c == nil {
		return Inventory{}
	}
	return Inventory{Documents: len(c.documents), Bytes: c.bytes}
}

// Open admits all metadata before I/O, then verifies every selected body. Any
// failure returns nil, never a smaller corpus. root is borrowed through return.
// Each owned string preserves the exact bytes; malformed UTF-8 and NUL fail.
// A deadline is required. Regular-file reads and hashing are not preemptive.
func Open(ctx context.Context, root *os.Root, pins []Pin, l OpenLimits) (*Corpus, error) {
	if err := checkContext(ctx); err != nil {
		return nil, err
	}
	if root == nil || l.MaxDocuments <= 0 || l.MaxDocumentBytes <= 0 || l.MaxDocumentBytes == int(^uint(0)>>1) || l.MaxCorpusBytes <= 0 || l.MaxMetadataBytes <= 0 {
		return nil, ErrInvalid
	}
	if len(pins) > l.MaxDocuments {
		return nil, ErrLimit
	}
	left := l.MaxMetadataBytes
	seen := make(map[string]bool, len(pins))
	for _, p := range pins {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if p.ID == "" || !utf8.ValidString(p.ID) || strings.ContainsRune(p.ID, 0) || p.Path == "." || !fs.ValidPath(p.Path) || strings.ContainsRune(p.Path, 0) || seen[p.ID] {
			return nil, ErrInvalid
		}
		if len(p.ID) > left {
			return nil, ErrLimit
		}
		left -= len(p.ID)
		if len(p.Path) > left {
			return nil, ErrLimit
		}
		left -= len(p.Path)
		seen[p.ID] = true
	}
	c := &Corpus{documents: make(map[string]document, len(pins))}
	for _, p := range pins {
		limit := min(l.MaxDocumentBytes, l.MaxCorpusBytes-c.bytes)
		body, err := readRegular(ctx, root, p.Path, limit)
		if err != nil {
			return nil, fmt.Errorf("read pinned text: %w", err)
		}
		if sha256.Sum256(body) != p.SHA256 {
			return nil, ErrBinding
		}
		if !utf8.Valid(body) || bytes.IndexByte(body, 0) >= 0 {
			return nil, ErrContent
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		id := strings.Clone(p.ID)
		// Conversion owns a copy independent of ReadAll's scratch buffer. Exported
		// read substrings may retain this one document, but cannot mutate it.
		c.documents[id] = document{ref: Ref{ID: id, SHA256: p.SHA256}, text: string(body)}
		c.bytes += len(body)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return c, nil
}

func checkContext(ctx context.Context) error {
	if ctx == nil {
		return ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, ok := ctx.Deadline(); !ok {
		return ErrInvalid
	}
	return nil
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p[:min(len(p), 32<<10)])
}

func readRegular(ctx context.Context, root *os.Root, path string, limit int) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Reject known links/devices/FIFOs before Open. The operator keeps the local
	// paths stable during loading; Lstat/Open is not a hostile-filesystem sandbox.
	before, err := root.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() {
		return nil, ErrInvalid
	}
	if before.Size() > int64(limit) {
		return nil, ErrLimit
	}
	f, err := root.Open(path)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		return nil, errors.Join(err, f.Close())
	}
	if !info.Mode().IsRegular() {
		return nil, errors.Join(ErrInvalid, f.Close())
	}
	if info.Size() > int64(limit) {
		return nil, errors.Join(ErrLimit, f.Close())
	}
	body, readErr := readBody(ctx, f, limit)
	if err := errors.Join(readErr, f.Close(), ctx.Err()); err != nil {
		return nil, err
	}
	if len(body) > limit {
		return nil, ErrLimit
	}
	return body, nil
}

// Size metadata can change before/during reading. The extra byte distinguishes
// an exact-fit body from a grown stream even when Lstat/Stat previously admitted it.
func readBody(ctx context.Context, r io.Reader, limit int) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(contextReader{ctx: ctx, r: r}, int64(limit)+1))
	if err != nil {
		return nil, err
	}
	if len(body) > limit {
		return nil, ErrLimit
	}
	return body, nil
}
