package capturefiles

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
	"strconv"
	"unicode/utf8"

	"github.com/sidsharma96/concordance/internal/htmlarticle"
)

type parser struct {
	ctx       context.Context
	d         *json.Decoder
	admission admission
}

func (p *parser) next() (json.Token, error) {
	if err := p.ctx.Err(); err != nil {
		return nil, err
	}
	t, err := p.d.Token()
	if err != nil {
		return nil, ErrInvalid
	}
	return t, nil
}
func (p *parser) delim(want json.Delim) error {
	t, err := p.next()
	if err != nil {
		return err
	}
	if t != want {
		return ErrInvalid
	}
	return nil
}
func (p *parser) text() (string, error) {
	t, err := p.next()
	if err != nil {
		return "", err
	}
	s, ok := t.(string)
	if !ok {
		return "", ErrInvalid
	}
	if err := p.admission.text(s); err != nil {
		return "", err
	}
	return s, nil
}

// Every schema object has at most nine keys; decoded escaped duplicates compare
// equal. Required-key masks reject omitted zero/default fields as well as nulls.
func (p *parser) object(keys []string, field func(int) error) error {
	if err := p.delim('{'); err != nil {
		return err
	}
	var seen uint16
	for p.d.More() {
		t, err := p.next()
		if err != nil {
			return err
		}
		key, ok := t.(string)
		if !ok {
			return ErrInvalid
		}
		at := -1
		for n, k := range keys {
			if k == key {
				at = n
				break
			}
		}
		if at < 0 || seen&(1<<at) != 0 {
			return ErrInvalid
		}
		seen |= 1 << at
		if err := field(at); err != nil {
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
func (p *parser) array(left *int, element func() error) error {
	if err := p.delim('['); err != nil {
		return err
	}
	for p.d.More() {
		if err := take(1, left); err != nil {
			return err
		}
		if err := element(); err != nil {
			return err
		}
	}
	return p.delim(']')
}
func (p *parser) strings(left *int) ([]string, error) {
	out := []string{}
	err := p.array(left, func() error {
		s, err := p.text()
		if err != nil {
			return err
		}
		out = append(out, s)
		return nil
	})
	return out, err
}
func (p *parser) number() (int64, error) {
	t, err := p.next()
	if err != nil {
		return 0, err
	}
	n, ok := t.(json.Number)
	if !ok {
		return 0, ErrInvalid
	}
	s := n.String()
	if s == "" || len(s) > 1 && s[0] == '0' {
		return 0, ErrInvalid
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, ErrInvalid
		}
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, ErrInvalid
	}
	return v, nil
}
func (p *parser) file() (out File, err error) {
	if err = take(1, &p.admission.files); err != nil {
		return
	}
	err = p.object([]string{"path", "sha256", "bytes"}, func(i int) error {
		switch i {
		case 0:
			out.Path, err = p.text()
		case 1:
			var s string
			s, err = p.text()
			if err != nil {
				return err
			}
			if len(s) != 64 || !objectID(s) {
				return ErrInvalid
			}
			b, e := hex.DecodeString(s)
			if e != nil {
				return ErrInvalid
			}
			copy(out.SHA256[:], b)
		case 2:
			out.Bytes, err = p.number()
		}
		return err
	})
	return
}
func (p *parser) profile() (out htmlarticle.Profile, err error) {
	err = p.object([]string{"id", "article_class", "omit_classes", "omit_ids", "notice_classes"}, func(i int) error {
		left := 32
		switch i {
		case 0:
			out.ID, err = p.text()
		case 1:
			out.ArticleClass, err = p.text()
		case 2:
			out.OmitClasses, err = p.strings(&left)
		case 3:
			out.OmitIDs, err = p.strings(&left)
		case 4:
			out.NoticeClasses, err = p.strings(&left)
		}
		return err
	})
	return
}
func (p *parser) page() (out Page, err error) {
	err = p.object([]string{"path", "html"}, func(i int) error {
		if i == 0 {
			out.Path, err = p.text()
		} else {
			out.HTML, err = p.file()
		}
		return err
	})
	return
}
func (p *parser) capture() (out Capture, err error) {
	err = p.object([]string{"source", "revision", "directory", "preparation", "toolchain", "recipe", "dependencies", "profile", "pages"}, func(i int) error {
		switch i {
		case 0:
			out.Source, err = p.text()
		case 1:
			out.Revision, err = p.text()
		case 2:
			out.Directory, err = p.text()
		case 3:
			out.Preparation, err = p.text()
		case 4:
			out.Toolchain, err = p.text()
		case 5:
			out.Recipe, err = p.file()
		case 6:
			out.Dependencies, err = p.strings(&p.admission.dependencies)
		case 7:
			out.Profile, err = p.profile()
		case 8:
			out.Pages = []Page{}
			err = p.array(&p.admission.pages, func() error {
				page, e := p.page()
				if e != nil {
					return e
				}
				out.Pages = append(out.Pages, page)
				return nil
			})
		}
		return err
	})
	return
}
func (p *parser) edition() (out Edition, err error) {
	err = p.object([]string{"id", "captures"}, func(i int) error {
		if i == 0 {
			out.ID, err = p.text()
		} else {
			out.Captures = []Capture{}
			err = p.array(&p.admission.captures, func() error {
				c, e := p.capture()
				if e != nil {
					return e
				}
				out.Captures = append(out.Captures, c)
				return nil
			})
		}
		return err
	})
	return
}
func decode(ctx context.Context, body []byte, l Limits) (Description, error) {
	if len(body) > l.MaxDescriptorBytes {
		return Description{}, ErrLimit
	}
	if !utf8.Valid(body) {
		return Description{}, ErrInvalid
	}
	if err := surrogates(ctx, body); err != nil {
		return Description{}, err
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	p := parser{ctx: ctx, d: dec, admission: newAdmission(l)}
	out := Description{Editions: []Edition{}}
	err := p.object([]string{"format", "editions"}, func(i int) error {
		if i == 0 {
			s, e := p.text()
			if e != nil {
				return e
			}
			if s != Format {
				return ErrUnsupported
			}
			return nil
		}
		return p.array(&p.admission.editions, func() error {
			ed, e := p.edition()
			if e != nil {
				return e
			}
			out.Editions = append(out.Editions, ed)
			return nil
		})
	})
	if err != nil {
		return Description{}, err
	}
	if err := ctx.Err(); err != nil {
		return Description{}, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return Description{}, ErrInvalid
	}
	return out, nil
}
