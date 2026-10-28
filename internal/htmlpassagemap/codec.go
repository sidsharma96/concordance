package htmlpassagemap

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/sidsharma96/concordance/internal/htmlpassage"
)

type header struct {
	Format           string `json:"format"`
	ArticleSHA256    string `json:"article_sha256"`
	ArticleBytes     int    `json:"article_bytes"`
	PassageProcessor string `json:"passage_processor"`
	FieldProcessor   string `json:"field_processor"`
	TermsVersion     string `json:"terms_version"`
	FieldsSHA256     string `json:"fields_sha256"`
	BlockCount       int    `json:"block_count"`
}
type row struct {
	First int `json:"first"`
	After int `json:"after"`
}

// Encode returns owned compact JSON with fixed field order and no trailing newline.
// It borrows the table unchanged until return. Any error returns nil. A small record
// is marshaled at a time; output bytes are bounded before append, not exact heap.
func Encode(ctx context.Context, t *Table, l Limits) ([]byte, error) {
	if err := ready(ctx, l); err != nil {
		return nil, err
	}
	if err := validate(ctx, t, l); err != nil {
		return nil, err
	}
	var out []byte
	add := func(b []byte) error {
		if len(b) > l.MaxEncodedBytes-len(out) {
			return ErrLimit
		}
		out = append(out, b...)
		return nil
	}
	h := header{Format, hex.EncodeToString(t.articleHash[:]), t.articleBytes, passageVersion, fieldVersion, termsVersion, hex.EncodeToString(t.fieldsHash[:]), t.blocks}
	b, err := json.Marshal(h)
	if err != nil {
		return nil, ErrInvalid
	}
	if err = add(b[:len(b)-1]); err != nil {
		return nil, err
	}
	if err = add([]byte(`,"locations":[`)); err != nil {
		return nil, err
	}
	for i, r := range t.rows {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		if i > 0 {
			if err = add([]byte(",")); err != nil {
				return nil, err
			}
		}
		b, err = json.Marshal(row{r.First, r.After})
		if err != nil {
			return nil, ErrInvalid
		}
		if err = add(b); err != nil {
			return nil, err
		}
	}
	if err = add([]byte(`]}`)); err != nil {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

type parser struct {
	ctx context.Context
	d   *json.Decoder
}

func (p *parser) next() (json.Token, error) {
	if err := p.ctx.Err(); err != nil {
		return nil, err
	}
	v, err := p.d.Token()
	if err != nil {
		if err := p.ctx.Err(); err != nil {
			return nil, err
		}
		return nil, ErrInvalid
	}
	return v, nil
}
func (p *parser) delim(want json.Delim) error {
	v, err := p.next()
	if err != nil {
		return err
	}
	if v != want {
		return ErrInvalid
	}
	return nil
}
func (p *parser) text() (string, error) {
	v, err := p.next()
	if err != nil {
		return "", err
	}
	s, ok := v.(string)
	if !ok {
		return "", ErrInvalid
	}
	// v1 strings contain only fixed ASCII version tags or lowercase hex digests.
	// A replacement rune introduced by permissive Unicode repair cannot pass here.
	if len(s) > 128 {
		return "", ErrInvalid
	}
	for i := range len(s) {
		if s[i] < 0x20 || s[i] > 0x7e {
			return "", ErrInvalid
		}
	}
	return s, nil
}
func (p *parser) number() (int, error) {
	v, err := p.next()
	if err != nil {
		return 0, err
	}
	n, ok := v.(json.Number)
	if !ok {
		return 0, ErrInvalid
	}
	s := n.String()
	if len(s) == 0 {
		return 0, ErrInvalid
	}
	for i := range len(s) {
		if s[i] < '0' || s[i] > '9' {
			return 0, ErrInvalid
		}
	}
	if len(s) > 1 && s[0] == '0' {
		return 0, ErrInvalid
	}
	u, err := strconv.ParseUint(s, 10, strconv.IntSize-1)
	if err != nil {
		return 0, ErrInvalid
	}
	return int(u), nil
}
func (p *parser) object(keys []string, field func(int) error) error {
	if err := p.delim('{'); err != nil {
		return err
	}
	var seen uint16
	for p.d.More() {
		key, err := p.text()
		if err != nil {
			return err
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
		if err := field(index); err != nil {
			return err
		}
	}
	if err := p.delim('}'); err != nil {
		return err
	}
	if seen != (1<<len(keys))-1 {
		return ErrInvalid
	}
	return nil
}
func (p *parser) digest() ([32]byte, error) {
	var sum [32]byte
	s, err := p.text()
	if err != nil {
		return sum, err
	}
	if len(s) != 64 || strings.ToLower(s) != s {
		return sum, ErrInvalid
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return sum, ErrInvalid
	}
	copy(sum[:], b)
	return sum, nil
}

func decode(ctx context.Context, body []byte, l Limits) (*Table, error) {
	if err := ready(ctx, l); err != nil {
		return nil, err
	}
	if len(body) > l.MaxEncodedBytes {
		return nil, ErrLimit
	}
	if !utf8.Valid(body) {
		return nil, ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(body))
	d.UseNumber()
	p := parser{ctx, d}
	t := &Table{}
	err := p.object([]string{"format", "article_sha256", "article_bytes", "passage_processor", "field_processor", "terms_version", "fields_sha256", "block_count", "locations"}, func(i int) error {
		var err error
		switch i {
		case 0, 3, 4, 5:
			var s string
			s, err = p.text()
			if err != nil {
				return err
			}
			want := [6]string{Format, "", "", passageVersion, fieldVersion, termsVersion}[i]
			if s != want {
				return ErrUnsupported
			}
		case 1:
			t.articleHash, err = p.digest()
		case 2:
			t.articleBytes, err = p.number()
		case 6:
			t.fieldsHash, err = p.digest()
		case 7:
			t.blocks, err = p.number()
		case 8:
			if err = p.delim('['); err != nil {
				return err
			}
			for d.More() {
				if len(t.rows) >= l.MaxPassages {
					return ErrLimit
				}
				var r htmlpassage.Range
				err = p.object([]string{"first", "after"}, func(i int) error {
					var err error
					if i == 0 {
						r.First, err = p.number()
					} else {
						r.After, err = p.number()
					}
					return err
				})
				if err != nil {
					return err
				}
				t.rows = append(t.rows, r)
			}
			err = p.delim(']')
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	if _, err := d.Token(); err != io.EOF {
		return nil, ErrInvalid
	}
	if err := validate(ctx, t, l); err != nil {
		return nil, err
	}
	return t, nil
}
