package generationmanifest

import (
	"context"
	"encoding/hex"
	"encoding/json"
)

// Dedicated fixed wire records keep exported Go DTO changes out of the schema.
type rootWire struct {
	Format        string `json:"format"`
	Collection    string `json:"collection"`
	Generation    string `json:"generation"`
	ArticleFormat string `json:"article_format"`
	IndexFormat   string `json:"index_format"`
}
type refWire struct {
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}
type sourceWire struct {
	ID         string `json:"id"`
	Repository string `json:"repository"`
}
type editionWire struct {
	ID    string  `json:"id"`
	Index refWire `json:"index"`
}
type captureWire struct {
	Source      string  `json:"source"`
	Revision    string  `json:"revision"`
	Directory   string  `json:"directory"`
	Preparation string  `json:"preparation"`
	Toolchain   string  `json:"toolchain"`
	Recipe      refWire `json:"recipe"`
}
type fileWire struct {
	Path   string  `json:"path"`
	BlobID string  `json:"blob_id"`
	Object refWire `json:"object"`
}
type documentWire struct {
	Capture  int     `json:"capture"`
	File     int     `json:"file"`
	NativeID string  `json:"native_id"`
	Location string  `json:"location"`
	Profile  string  `json:"profile"`
	HTML     refWire `json:"html"`
	Article  refWire `json:"article"`
}
type imageWire struct {
	Block   int    `json:"block"`
	Address string `json:"address"`
	File    int    `json:"file"`
}

func wireRef(r Ref) refWire { return refWire{hex.EncodeToString(r.SHA256[:]), r.Bytes} }

type writer struct {
	ctx  context.Context
	max  int
	body []byte
}

func (w *writer) add(b []byte) error {
	if err := w.ctx.Err(); err != nil {
		return err
	}
	if len(b) > w.max-len(w.body) {
		return ErrLimit
	}
	w.body = append(w.body, b...)
	return nil
}
func (w *writer) literal(s string) error { return w.add([]byte(s)) }
func (w *writer) record(v any, header bool) error {
	if err := w.ctx.Err(); err != nil {
		return err
	}
	b, err := json.Marshal(v)
	if err != nil {
		return ErrInvalid
	}
	if header {
		b = b[:len(b)-1]
	}
	return w.add(b)
}
func (w *writer) comma(i int) error {
	if i > 0 {
		return w.literal(",")
	}
	return nil
}

// Encode validates before encoding, then appends bounded individual wire records.
// Errors return nil, never a partial manifest. Nil lists encode as []. This fixed
// field order is versioned; list order remains meaningful, notably article slots.
// It is not a canonicalizer for logically equivalent unordered capture requests.
func Encode(ctx context.Context, d Description, l Limits) ([]byte, error) {
	if err := ready(ctx, l); err != nil {
		return nil, err
	}
	if _, err := validate(ctx, d, l); err != nil {
		return nil, err
	}
	w := writer{ctx: ctx, max: l.MaxEncodedBytes, body: []byte{}}
	if err := w.record(rootWire{Format, d.Collection, d.Generation, articleFormat, indexFormat}, true); err != nil {
		return nil, err
	}
	if err := w.literal(`,"sources":[`); err != nil {
		return nil, err
	}
	for i, s := range d.Sources {
		if err := w.comma(i); err != nil {
			return nil, err
		}
		if err := w.record(sourceWire{s.ID, s.Repository}, false); err != nil {
			return nil, err
		}
	}
	if err := w.literal(`],"editions":[`); err != nil {
		return nil, err
	}
	for i, e := range d.Editions {
		if err := w.comma(i); err != nil {
			return nil, err
		}
		if err := w.record(editionWire{e.ID, wireRef(e.Index)}, true); err != nil {
			return nil, err
		}
		if err := w.literal(`,"captures":[`); err != nil {
			return nil, err
		}
		for ci, c := range e.Captures {
			if err := w.comma(ci); err != nil {
				return nil, err
			}
			if err := w.record(captureWire{c.Source, c.Revision, c.Directory, c.Preparation, c.Toolchain, wireRef(c.Recipe)}, true); err != nil {
				return nil, err
			}
			if err := w.literal(`,"files":[`); err != nil {
				return nil, err
			}
			for fi, f := range c.Files {
				if err := w.comma(fi); err != nil {
					return nil, err
				}
				if err := w.record(fileWire{f.Path, f.BlobID, wireRef(f.Object)}, false); err != nil {
					return nil, err
				}
			}
			if err := w.literal(`]}`); err != nil {
				return nil, err
			}
		}
		if err := w.literal(`],"documents":[`); err != nil {
			return nil, err
		}
		for di, doc := range e.Documents {
			if err := w.comma(di); err != nil {
				return nil, err
			}
			if err := w.record(documentWire{doc.Capture, doc.File, doc.NativeID, doc.Location, doc.Profile, wireRef(doc.HTML), wireRef(doc.Article)}, true); err != nil {
				return nil, err
			}
			if err := w.literal(`,"images":[`); err != nil {
				return nil, err
			}
			for ii, im := range doc.Images {
				if err := w.comma(ii); err != nil {
					return nil, err
				}
				if err := w.record(imageWire{im.Block, im.Address, im.File}, false); err != nil {
					return nil, err
				}
			}
			if err := w.literal(`]}`); err != nil {
				return nil, err
			}
		}
		if err := w.literal(`]}`); err != nil {
			return nil, err
		}
	}
	if err := w.literal(`]}`); err != nil {
		return nil, err
	}
	return w.body, nil
}
