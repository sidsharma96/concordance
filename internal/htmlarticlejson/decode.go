package htmlarticlejson

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/sidsharma96/concordance/internal/htmlarticle"
)

type parser struct {
	ctx     context.Context
	decoder *json.Decoder
	values  values
}

func (p *parser) next() (json.Token, error) {
	if e := p.ctx.Err(); e != nil {
		return nil, e
	}
	v, e := p.decoder.Token()
	if e != nil {
		if e := p.ctx.Err(); e != nil {
			return nil, e
		}
		return nil, ErrInvalid
	}
	return v, nil
}
func (p *parser) delim(want json.Delim) error {
	v, e := p.next()
	if e != nil {
		return e
	}
	if v != want {
		return ErrInvalid
	}
	return nil
}
func (p *parser) text() (string, error) {
	v, e := p.next()
	if e != nil {
		return "", e
	}
	s, ok := v.(string)
	if !ok {
		return "", ErrInvalid
	}
	if e = p.values.admit(s); e != nil {
		return "", e
	}
	return s, nil
}
func (p *parser) number() (int, error) {
	v, e := p.next()
	if e != nil {
		return 0, e
	}
	n, ok := v.(json.Number)
	if !ok {
		return 0, ErrInvalid
	}
	s := n.String()
	if len(s) == 0 || len(s) > 1 && s[0] == '0' {
		return 0, ErrInvalid
	}
	for _, c := range []byte(s) {
		if c < '0' || c > '9' {
			return 0, ErrInvalid
		}
	}
	u, e := strconv.ParseUint(s, 10, strconv.IntSize-1)
	if e != nil {
		return 0, ErrInvalid
	}
	return int(u), nil
}
func (p *parser) boolean() (bool, error) {
	v, e := p.next()
	if e != nil {
		return false, e
	}
	b, ok := v.(bool)
	if !ok {
		return false, ErrInvalid
	}
	return b, nil
}

// All callers provide fixed schema keys (at most eight), so key search and the
// required/duplicate bitset are constant-sized. Decoded escaped keys compare equal.
func (p *parser) object(keys []string, field func(int) error) error {
	if e := p.delim('{'); e != nil {
		return e
	}
	var seen uint16
	for p.decoder.More() {
		v, e := p.next()
		if e != nil {
			return e
		}
		key, ok := v.(string)
		if !ok {
			return ErrInvalid
		}
		index := -1
		for i, k := range keys {
			if key == k {
				index = i
				break
			}
		}
		if index < 0 || seen&(1<<index) != 0 {
			return ErrInvalid
		}
		seen |= 1 << index
		if e = field(index); e != nil {
			return e
		}
	}
	if e := p.delim('}'); e != nil {
		return e
	}
	if seen != (1<<len(keys))-1 {
		return ErrInvalid
	}
	return nil
}
func (p *parser) block() (b htmlarticle.Block, e error) {
	e = p.object([]string{"kind", "text", "level", "anchor", "source_url"}, func(i int) error {
		var x error
		switch i {
		case 0:
			var s string
			s, x = p.text()
			b.Kind = htmlarticle.Kind(s)
		case 1:
			b.Text, x = p.text()
		case 2:
			b.Level, x = p.number()
		case 3:
			b.Anchor, x = p.text()
		case 4:
			b.SourceURL, x = p.text()
		}
		return x
	})
	return
}
func (p *parser) group() (g htmlarticle.Group, e error) {
	e = p.object([]string{"kind", "first", "after", "label", "ordered", "header"}, func(i int) error {
		var x error
		switch i {
		case 0:
			var s string
			s, x = p.text()
			g.Kind = htmlarticle.GroupKind(s)
		case 1:
			g.First, x = p.number()
		case 2:
			g.After, x = p.number()
		case 3:
			g.Label, x = p.text()
		case 4:
			g.Ordered, x = p.boolean()
		case 5:
			g.Header, x = p.boolean()
		}
		return x
	})
	return
}

// Decode borrows body unchanged until return, returning an independently owned
// article. Nil and empty groups normalize to nil. Strings preserve their decoded
// text exactly. Any error returns a zero article. It does no source-hash lookup.
func Decode(ctx context.Context, body []byte, l Limits) (htmlarticle.Article, error) {
	zero := htmlarticle.Article{}
	if e := ready(ctx, l); e != nil {
		return zero, e
	}
	if len(body) > l.MaxEncodedBytes {
		return zero, ErrLimit
	}
	if !utf8.Valid(body) {
		return zero, ErrInvalid
	}
	if e := surrogates(ctx, body); e != nil {
		return zero, e
	}
	d := json.NewDecoder(bytes.NewReader(body))
	d.UseNumber()
	p := parser{ctx, d, values{l, min(l.MaxValueBytes, l.MaxEncodedBytes)}}
	var a htmlarticle.Article
	e := p.object([]string{"format", "kind", "processor", "profile", "html_sha256", "html_bytes", "blocks", "groups"}, func(i int) error {
		var e error
		switch i {
		case 0, 1:
			var s string
			s, e = p.text()
			if e == nil && (i == 0 && s != Format || i == 1 && s != "html") {
				return ErrUnsupported
			}
		case 2:
			a.Processor, e = p.text()
		case 3:
			a.Profile, e = p.text()
		case 4:
			var s string
			s, e = p.text()
			if e != nil {
				return e
			}
			if len(s) != 64 || strings.ToLower(s) != s {
				return ErrInvalid
			}
			b, x := hex.DecodeString(s)
			if x != nil {
				return ErrInvalid
			}
			copy(a.HTMLSHA256[:], b)
		case 5:
			a.HTMLBytes, e = p.number()
		case 6:
			if e = p.delim('['); e != nil {
				return e
			}
			for d.More() {
				if len(a.Blocks) >= l.MaxBlocks {
					return ErrLimit
				}
				b, x := p.block()
				if x != nil {
					return x
				}
				a.Blocks = append(a.Blocks, b)
			}
			e = p.delim(']')
		case 7:
			if e = p.delim('['); e != nil {
				return e
			}
			for d.More() {
				if len(a.Groups) >= l.MaxGroups {
					return ErrLimit
				}
				g, x := p.group()
				if x != nil {
					return x
				}
				a.Groups = append(a.Groups, g)
			}
			e = p.delim(']')
		}
		return e
	})
	if e != nil {
		return zero, e
	}
	if _, e = d.Token(); e != io.EOF {
		return zero, ErrInvalid
	}
	if e = validate(ctx, a, l); e != nil {
		return zero, e
	}
	if e = ctx.Err(); e != nil {
		return zero, e
	}
	return a, nil
}
