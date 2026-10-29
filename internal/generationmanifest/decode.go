package generationmanifest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"strconv"
	"unicode/utf8"
)

type parser struct {
	ctx                                                   context.Context
	d                                                     *json.Decoder
	values                                                admission
	sources, editions, captures, files, documents, images int
}

func (p *parser) next() (json.Token, error) {
	if err := p.ctx.Err(); err != nil {
		return nil, err
	}
	t, err := p.d.Token()
	if err != nil {
		if err := p.ctx.Err(); err != nil {
			return nil, err
		}
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
	if err := p.values.text(s); err != nil {
		return "", err
	}
	return s, nil
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
	for _, c := range []byte(s) {
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
func (p *parser) index() (int, error) {
	n, err := p.number()
	if err != nil {
		return 0, err
	}
	if n > int64(^uint(0)>>1) {
		return 0, ErrInvalid
	}
	return int(n), nil
}

// Every schema object has at most eight exact keys. Escaped keys compare after
// decoding; duplicate aliases are rejected. No recursive arbitrary-object parsing.
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
		for i, k := range keys {
			if key == k {
				at = i
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
func (p *parser) ref() (r Ref, err error) {
	err = p.object([]string{"sha256", "bytes"}, func(i int) error {
		if i == 1 {
			r.Bytes, err = p.number()
			return err
		}
		var s string
		s, err = p.text()
		if err != nil {
			return err
		}
		if !gitID(s, 64) {
			return ErrInvalid
		}
		var b []byte
		b, err = hex.DecodeString(s)
		if err != nil {
			return ErrInvalid
		}
		copy(r.SHA256[:], b)
		return nil
	})
	return
}
func (p *parser) source() (s Source, err error) {
	err = p.object([]string{"id", "repository"}, func(i int) error {
		if i == 0 {
			s.ID, err = p.text()
		} else {
			s.Repository, err = p.text()
		}
		return err
	})
	return
}
func (p *parser) file() (f File, err error) {
	err = p.object([]string{"path", "blob_id", "object"}, func(i int) error {
		switch i {
		case 0:
			f.Path, err = p.text()
		case 1:
			f.BlobID, err = p.text()
		case 2:
			f.Object, err = p.ref()
		}
		return err
	})
	return
}
func (p *parser) capture() (c Capture, err error) {
	err = p.object([]string{"source", "revision", "directory", "preparation", "toolchain", "recipe", "files"}, func(i int) error {
		switch i {
		case 0:
			c.Source, err = p.text()
		case 1:
			c.Revision, err = p.text()
		case 2:
			c.Directory, err = p.text()
		case 3:
			c.Preparation, err = p.text()
		case 4:
			c.Toolchain, err = p.text()
		case 5:
			c.Recipe, err = p.ref()
		case 6:
			err = p.array(&p.files, func() error {
				f, e := p.file()
				if e != nil {
					return e
				}
				c.Files = append(c.Files, f)
				return nil
			})
		}
		return err
	})
	return
}
func (p *parser) image() (im Image, err error) {
	err = p.object([]string{"block", "address", "file"}, func(i int) error {
		switch i {
		case 0:
			im.Block, err = p.index()
		case 1:
			im.Address, err = p.text()
		case 2:
			im.File, err = p.index()
		}
		return err
	})
	return
}
func (p *parser) document() (doc Document, err error) {
	err = p.object([]string{"capture", "file", "native_id", "location", "profile", "html", "article", "images"}, func(i int) error {
		switch i {
		case 0:
			doc.Capture, err = p.index()
		case 1:
			doc.File, err = p.index()
		case 2:
			doc.NativeID, err = p.text()
		case 3:
			doc.Location, err = p.text()
		case 4:
			doc.Profile, err = p.text()
		case 5:
			doc.HTML, err = p.ref()
		case 6:
			doc.Article, err = p.ref()
		case 7:
			err = p.array(&p.images, func() error {
				im, e := p.image()
				if e != nil {
					return e
				}
				doc.Images = append(doc.Images, im)
				return nil
			})
		}
		return err
	})
	return
}
func (p *parser) edition() (ed Edition, err error) {
	err = p.object([]string{"id", "index", "captures", "documents"}, func(i int) error {
		switch i {
		case 0:
			ed.ID, err = p.text()
		case 1:
			ed.Index, err = p.ref()
		case 2:
			err = p.array(&p.captures, func() error {
				c, e := p.capture()
				if e != nil {
					return e
				}
				ed.Captures = append(ed.Captures, c)
				return nil
			})
		case 3:
			err = p.array(&p.documents, func() error {
				d, e := p.document()
				if e != nil {
					return e
				}
				ed.Documents = append(ed.Documents, d)
				return nil
			})
		}
		return err
	})
	return
}
func (p *parser) description() (d Description, err error) {
	err = p.object([]string{"format", "collection", "generation", "article_format", "index_format", "sources", "editions"}, func(i int) error {
		switch i {
		case 0, 3, 4:
			var s string
			s, err = p.text()
			if err != nil {
				return err
			}
			want := Format
			if i == 3 {
				want = articleFormat
			}
			if i == 4 {
				want = indexFormat
			}
			if s != want {
				return ErrUnsupported
			}
		case 1:
			d.Collection, err = p.text()
		case 2:
			d.Generation, err = p.text()
		case 5:
			err = p.array(&p.sources, func() error {
				s, e := p.source()
				if e != nil {
					return e
				}
				d.Sources = append(d.Sources, s)
				return nil
			})
		case 6:
			err = p.array(&p.editions, func() error {
				e, x := p.edition()
				if x != nil {
					return x
				}
				d.Editions = append(d.Editions, e)
				return nil
			})
		}
		return err
	})
	return
}

// Open checks independently trusted bytes and collection/generation pins before
// returning owned immutable metadata. Supplying a digest oneself grants no authority.
// Token allocations are bounded by admitted input bytes; smaller logical limits
// apply before growing output lists, not before json.Decoder allocates each token.
func Open(ctx context.Context, body []byte, expected Ref, collection, generation string, l Limits) (*Manifest, error) {
	if err := ready(ctx, l); err != nil {
		return nil, err
	}
	if !identifier(collection) || !identifier(generation) || expected.Bytes <= 0 {
		return nil, ErrInvalid
	}
	if len(body) > l.MaxEncodedBytes || expected.Bytes > int64(l.MaxEncodedBytes) {
		return nil, ErrLimit
	}
	if int64(len(body)) != expected.Bytes || sha256.Sum256(body) != expected.SHA256 {
		return nil, ErrMismatch
	}
	if !utf8.Valid(body) {
		return nil, ErrInvalid
	}
	if err := surrogates(ctx, body); err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	p := parser{ctx: ctx, d: decoder, values: newAdmission(l), sources: l.MaxSources, editions: l.MaxEditions, captures: l.MaxCaptures, files: l.MaxFiles, documents: l.MaxDocuments, images: l.MaxImages}
	d, err := p.description()
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if _, err = decoder.Token(); err != io.EOF {
		return nil, ErrInvalid
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if d.Collection != collection || d.Generation != generation {
		return nil, ErrMismatch
	}
	objects, err := validate(ctx, d, l)
	if err != nil {
		return nil, err
	}
	return &Manifest{description: d, objects: objects}, nil
}
