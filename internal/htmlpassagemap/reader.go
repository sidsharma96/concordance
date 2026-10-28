package htmlpassagemap

import (
	"context"
	"crypto/sha256"
	"strings"

	"github.com/sidsharma96/concordance/internal/htmlarticle"
	"github.com/sidsharma96/concordance/internal/htmlarticlejson"
	"github.com/sidsharma96/concordance/internal/htmlpassage"
)

// Reader owns a decoded article and saved ranges. No caller input is retained and
// no mutable article views escape. Read-only calls may run concurrently. It neither
// searches nor authorizes: callers must use the exact checked fields and preserve
// generation/edition/document associations separately. The zero value is invalid.
type Reader struct {
	table   *Table
	article htmlarticle.Article
}

// Open checks exact article bytes, versions, field contents/order and range bounds
// before exposing a reader. It never calls a passage builder or prepares terms.
// All inputs are borrowed unchanged until return. Errors return nil. Matching
// bindings detect mixed artifacts, not a malicious writer forging all artifacts.
func Open(ctx context.Context, locationJSON, articleJSON []byte, f Fields, l OpenLimits) (*Reader, error) {
	if err := ready(ctx, l.Locations); err != nil {
		return nil, err
	}
	if !articleLimitsValid(l.Article) {
		return nil, ErrInvalid
	}
	t, err := decode(ctx, locationJSON, l.Locations)
	if err != nil {
		return nil, err
	}
	if len(articleJSON) > l.Article.MaxEncodedBytes {
		return nil, ErrLimit
	}
	if len(articleJSON) != t.articleBytes || sha256.Sum256(articleJSON) != t.articleHash {
		return nil, ErrMismatch
	}
	a, err := htmlarticlejson.Decode(ctx, articleJSON, l.Article)
	if err != nil {
		return nil, err
	}
	if len(a.Blocks) != t.blocks || len(f.Passages) != len(t.rows) {
		return nil, ErrMismatch
	}
	hash, err := fingerprint(ctx, f, l.Locations)
	if err != nil {
		return nil, err
	}
	if hash != t.fieldsHash {
		return nil, ErrMismatch
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &Reader{t, a}, nil
}

// Location returns an owned scalar range within this reader's exact article.
// A passage position is local to this table, not a durable standalone citation.
func (r *Reader) Location(position int) (htmlpassage.Range, error) {
	if r == nil || r.table == nil || position < 0 || position >= len(r.table.rows) {
		return htmlpassage.Range{}, ErrInvalid
	}
	return r.table.rows[position], nil
}

// Quote returns an owned string from saved body blocks joined by LF. It never adds
// heading context, notice labels or image URLs. Count before allocating, then copy;
// errors return an empty string. Work is O(blocks+bytes), output O(bytes). No I/O or
// goroutines start. Context checks do not preempt a library call.
func (r *Reader) Quote(ctx context.Context, position, maxBytes int) (string, error) {
	if ctx == nil {
		return "", ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if _, ok := ctx.Deadline(); !ok || maxBytes <= 0 {
		return "", ErrInvalid
	}
	location, err := r.Location(position)
	if err != nil {
		return "", err
	}
	blocks := r.article.Blocks[location.First:location.After]
	remaining := maxBytes
	for i, b := range blocks {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if i > 0 {
			if remaining == 0 {
				return "", ErrLimit
			}
			remaining--
		}
		if len(b.Text) > remaining {
			return "", ErrLimit
		}
		remaining -= len(b.Text)
	}
	var text strings.Builder
	text.Grow(maxBytes - remaining)
	for i, b := range blocks {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if i > 0 {
			text.WriteByte('\n')
		}
		text.WriteString(b.Text)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return text.String(), nil
}
