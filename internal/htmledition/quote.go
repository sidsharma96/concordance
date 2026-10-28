package htmledition

import (
	"context"
	"crypto/sha256"
	"strings"

	"github.com/sidsharma96/concordance/internal/htmlarticlejson"
)

// Quote verifies and decodes this location's exact retained article, then returns
// an owned LF-joined body string. It does not fetch or retain article bytes, add
// context outside the body, rebuild passages or assert source identity. Caller
// aliases stay unchanged until return. Article decode is bounded separately from
// output; each quote decodes the complete supplied article. No cache or I/O.
func (e *Edition) Quote(ctx context.Context, position int, articleJSON []byte, l htmlarticlejson.Limits, maxBytes int) (string, error) {
	if ctx == nil {
		return "", ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if _, ok := ctx.Deadline(); !ok || maxBytes <= 0 || !articleLimitsValid(l) {
		return "", ErrInvalid
	}
	location, err := e.Location(position)
	if err != nil {
		return "", err
	}
	a := e.articles[location.Article]
	if len(articleJSON) > l.MaxEncodedBytes {
		return "", ErrLimit
	}
	if len(articleJSON) != a.Object.Bytes || sha256.Sum256(articleJSON) != a.Object.SHA256 {
		return "", ErrMismatch
	}
	decoded, err := htmlarticlejson.Decode(ctx, articleJSON, l)
	if err != nil {
		return "", err
	}
	if len(decoded.Blocks) != a.Blocks {
		return "", ErrMismatch
	}
	remaining := maxBytes
	blocks := decoded.Blocks[location.Body.First:location.Body.After]
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
	var out strings.Builder
	out.Grow(maxBytes - remaining)
	for i, b := range blocks {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if i > 0 {
			out.WriteByte('\n')
		}
		out.WriteString(b.Text)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return out.String(), nil
}
