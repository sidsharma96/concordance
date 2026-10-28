package htmlarticlejson

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/sidsharma96/concordance/internal/htmlarticle"
)

type values struct {
	limits    Limits
	remaining int
}

func (v *values) admit(s string) error {
	if len(s) > v.limits.MaxStringBytes || len(s) > v.remaining {
		return ErrLimit
	}
	if !utf8.ValidString(s) || strings.IndexByte(s, 0) >= 0 {
		return ErrInvalid
	}
	v.remaining -= len(s)
	return nil
}

// This v1 validator belongs to the persisted schema, independently of changes to
// current passage grouping. It checks representation, not full HTML grammar or
// provenance. A claimed input hash does not establish the original HTML is retained.
func validate(ctx context.Context, a htmlarticle.Article, l Limits) error {
	if len(a.Blocks) == 0 || a.HTMLBytes <= 0 {
		return ErrInvalid
	}
	if len(a.Blocks) > l.MaxBlocks || len(a.Groups) > l.MaxGroups || a.HTMLBytes > l.MaxHTMLBytes {
		return ErrLimit
	}
	if a.Processor != articleProcessor {
		return ErrUnsupported
	}
	if !profile(a.Profile) {
		return ErrInvalid
	}
	v := values{l, min(l.MaxValueBytes, l.MaxEncodedBytes)}
	// The lowercase hex digest has exactly 64 ASCII bytes.
	for _, s := range []string{Format, "html", a.Processor, a.Profile, strings.Repeat("0", 64)} {
		if e := v.admit(s); e != nil {
			return e
		}
	}
	for _, b := range a.Blocks {
		if e := ctx.Err(); e != nil {
			return e
		}
		for _, s := range []string{string(b.Kind), b.Text, b.Anchor, b.SourceURL} {
			if e := v.admit(s); e != nil {
				return e
			}
		}
		switch string(b.Kind) {
		case "heading":
			if b.Level < 1 || b.Level > 6 {
				return ErrInvalid
			}
		case "prose", "code", "image", "separator":
			if b.Level != 0 {
				return ErrInvalid
			}
		default:
			return ErrUnsupported
		}
		if b.Kind != "image" && b.SourceURL != "" {
			return ErrInvalid
		}
	}
	previous := -1
	var ends []int
	for _, g := range a.Groups {
		if e := ctx.Err(); e != nil {
			return e
		}
		if e := v.admit(string(g.Kind)); e != nil {
			return e
		}
		if e := v.admit(g.Label); e != nil {
			return e
		}
		switch string(g.Kind) {
		case "paragraph", "notice", "list", "list-item", "definition-list", "term", "definition", "table", "row", "cell", "figure", "caption", "quotation":
		default:
			return ErrUnsupported
		}
		if g.First < 0 || g.After < g.First || g.After > len(a.Blocks) || g.First < previous || g.Ordered && g.Kind != "list" || g.Header && g.Kind != "cell" || g.Label != "" && g.Kind != "notice" {
			return ErrInvalid
		}
		previous = g.First
		for len(ends) > 0 && g.First >= ends[len(ends)-1] {
			ends = ends[:len(ends)-1]
		}
		if len(ends) > 0 && g.After > ends[len(ends)-1] {
			return ErrInvalid
		}
		if g.First != g.After {
			if len(ends) >= l.MaxGroupDepth {
				return ErrLimit
			}
			ends = append(ends, g.After)
		}
	}
	return ctx.Err()
}
func profile(s string) bool {
	if len(s) == 0 || len(s) > 128 {
		return false
	}
	for _, b := range []byte(s) {
		if !(b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '_' || b == '/' || b == '.' || b == '-') {
			return false
		}
	}
	return true
}
