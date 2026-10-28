// Package htmlarticlejson encodes bounded retained HTML articles using an explicit
// versioned JSON schema. It does not acquire source, persist citations, authorize
// access, store objects or publish generations.
package htmlarticlejson

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/sidsharma96/concordance/internal/htmlarticle"
)

const Format = "concordance.article/v1"
const articleProcessor = "htmlarticle/v1"

var (
	ErrInvalid     = errors.New("invalid article JSON")
	ErrLimit       = errors.New("article JSON limit exceeded")
	ErrUnsupported = errors.New("unsupported article JSON format or content")
)

// Limits bound logical input/output, records and decoded string VALUES (not keys).
// They are not exact heap or CPU quotas. All limits must be positive. A JSON token
// or encoded record may be allocated before its smaller output/value limit is
// checked, bounded by admitted input bytes or admitted record strings respectively.
type Limits struct {
	MaxEncodedBytes int
	MaxBlocks       int
	MaxGroups       int
	MaxStringBytes  int
	MaxValueBytes   int
	MaxGroupDepth   int
	MaxHTMLBytes    int
}

func ready(ctx context.Context, l Limits) error {
	if ctx == nil {
		return ErrInvalid
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	if _, ok := ctx.Deadline(); !ok {
		return ErrInvalid
	}
	if l.MaxEncodedBytes <= 0 || l.MaxBlocks <= 0 || l.MaxGroups <= 0 || l.MaxStringBytes <= 0 || l.MaxValueBytes <= 0 || l.MaxGroupDepth <= 0 || l.MaxHTMLBytes <= 0 {
		return ErrInvalid
	}
	return nil
}

// Dedicated v1 wire types prevent exported Go fields from becoming schema changes.
// Every field is required, including empty strings, false flags and zero levels.
type metadata struct {
	Format     string `json:"format"`
	Kind       string `json:"kind"`
	Processor  string `json:"processor"`
	Profile    string `json:"profile"`
	HTMLSHA256 string `json:"html_sha256"`
	HTMLBytes  int    `json:"html_bytes"`
}
type block struct {
	Kind      string `json:"kind"`
	Text      string `json:"text"`
	Level     int    `json:"level"`
	Anchor    string `json:"anchor"`
	SourceURL string `json:"source_url"`
}
type group struct {
	Kind    string `json:"kind"`
	First   int    `json:"first"`
	After   int    `json:"after"`
	Label   string `json:"label"`
	Ordered bool   `json:"ordered"`
	Header  bool   `json:"header"`
}

// Encode borrows all article aliases unchanged until return and produces owned
// compact bytes in fixed field order, without a trailing newline. Empty/nil groups
// both encode as []. Errors return nil, never a partially encoded article.
func Encode(ctx context.Context, a htmlarticle.Article, l Limits) ([]byte, error) {
	if e := ready(ctx, l); e != nil {
		return nil, e
	}
	if e := validate(ctx, a, l); e != nil {
		return nil, e
	}
	out := make([]byte, 0)
	appendBytes := func(b []byte) error {
		if len(b) > l.MaxEncodedBytes-len(out) {
			return ErrLimit
		}
		out = append(out, b...)
		return nil
	}
	record := func(v any, trim bool) error {
		if e := ctx.Err(); e != nil {
			return e
		}
		b, e := json.Marshal(v)
		if e != nil {
			return ErrInvalid
		}
		if trim {
			b = b[:len(b)-1]
		}
		return appendBytes(b)
	}
	m := metadata{Format, "html", a.Processor, a.Profile, hex.EncodeToString(a.HTMLSHA256[:]), a.HTMLBytes}
	if e := record(m, true); e != nil {
		return nil, e
	}
	if e := appendBytes([]byte(`,"blocks":[`)); e != nil {
		return nil, e
	}
	for i, b := range a.Blocks {
		if i > 0 {
			if e := appendBytes([]byte(",")); e != nil {
				return nil, e
			}
		}
		if e := record(block{string(b.Kind), b.Text, b.Level, b.Anchor, b.SourceURL}, false); e != nil {
			return nil, e
		}
	}
	if e := appendBytes([]byte(`],"groups":[`)); e != nil {
		return nil, e
	}
	for i, g := range a.Groups {
		if i > 0 {
			if e := appendBytes([]byte(",")); e != nil {
				return nil, e
			}
		}
		if e := record(group{string(g.Kind), g.First, g.After, g.Label, g.Ordered, g.Header}, false); e != nil {
			return nil, e
		}
	}
	if e := appendBytes([]byte(`]}`)); e != nil {
		return nil, e
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	return out, nil
}
