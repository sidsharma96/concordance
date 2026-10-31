package generationmanifest

import (
	"context"
	"crypto/sha256"

	"github.com/sidsharma96/concordance/internal/htmlarticle"
	"github.com/sidsharma96/concordance/internal/htmlarticlejson"
	"github.com/sidsharma96/concordance/internal/htmledition"
)

func (m *Manifest) edition(ctx context.Context, id string) (*Edition, error) {
	if err := contextReady(ctx); err != nil {
		return nil, err
	}
	if !m.valid() || !identifier(id) {
		return nil, ErrInvalid
	}
	for i := range m.description.Editions {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if m.description.Editions[i].ID == id {
			return &m.description.Editions[i], nil
		}
	}
	return nil, ErrNotFound
}

// Binding is the logical/source association for one article slot, not a persisted
// passage ID or permission. It owns its image slice; strings are immutable values.
// Source.Repository is the captured observation, never a lookup of current config.
type Binding struct {
	Source                                      Source
	Revision, Directory, Preparation, Toolchain string
	Recipe                                      Ref
	NativeID, Location, Profile                 string
	Input                                       File
	HTML, Article                               Ref
	Images                                      []BoundImage
}
type BoundImage struct {
	Block   int
	Address string
	Input   File
}

// Resolve copies only one slot's metadata, avoiding a full Describe copy per hit.
// Work is O(editions + sources + this document's images), not all document entries.
// Keep its generation/edition/slot association with the opened index; the content
// digest alone is not the logical source identity or authorization.
func (m *Manifest) Resolve(ctx context.Context, id string, slot int) (Binding, error) {
	e, err := m.edition(ctx, id)
	if err != nil {
		return Binding{}, err
	}
	if slot < 0 || slot >= len(e.Documents) {
		return Binding{}, ErrInvalid
	}
	d := e.Documents[slot]
	c := e.Captures[d.Capture]
	out := Binding{Revision: c.Revision, Directory: c.Directory, Preparation: c.Preparation, Toolchain: c.Toolchain, Recipe: c.Recipe, NativeID: d.NativeID, Location: d.Location, Profile: d.Profile, Input: c.Files[d.File], HTML: d.HTML, Article: d.Article, Images: make([]BoundImage, len(d.Images))}
	for _, source := range m.description.Sources {
		if err := ctx.Err(); err != nil {
			return Binding{}, err
		}
		if source.ID == c.Source {
			out.Source = source
			break
		}
	}
	for i, im := range d.Images {
		if err := ctx.Err(); err != nil {
			return Binding{}, err
		}
		out.Images[i] = BoundImage{Block: im.Block, Address: im.Address, Input: c.Files[im.File]}
	}
	if err := ctx.Err(); err != nil {
		return Binding{}, err
	}
	return out, nil
}

// EditionIndex returns the selected edition's expected index reference by value.
// Missing and known-empty editions stay distinct: an empty edition still has an
// index to read. This does no object I/O and exposes no mutable metadata slices.
func (m *Manifest) EditionIndex(ctx context.Context, id string) (Ref, error) {
	e, err := m.edition(ctx, id)
	if err != nil {
		return Ref{}, err
	}
	return e.Index, nil
}

// OpenEdition opens just the selected exact index, checks its article slots against
// the manifest and bounds saved image blocks against descriptors. No body is read;
// checking one index doesn't verify other indexes, source provenance or existence.
// The returned index owns its state. No reference to caller bytes is retained.
func (m *Manifest) OpenEdition(ctx context.Context, id string, body []byte, l htmledition.Limits) (*htmledition.Edition, error) {
	e, err := m.edition(ctx, id)
	if err != nil {
		return nil, err
	}
	// Manifest lengths are int64; the index API uses native int. Never wrap on 32-bit.
	if e.Index.Bytes > int64(^uint(0)>>1) {
		return nil, ErrLimit
	}
	x, err := htmledition.Open(ctx, body, htmledition.Ref{SHA256: e.Index.SHA256, Bytes: int(e.Index.Bytes)}, id, l)
	if err != nil {
		return nil, err
	}
	if x.ArticleCount() != len(e.Documents) {
		return nil, ErrMismatch
	}
	for i, doc := range e.Documents {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		a, err := x.Article(i)
		if err != nil {
			return nil, err
		}
		if a.Object.SHA256 != doc.Article.SHA256 || int64(a.Object.Bytes) != doc.Article.Bytes {
			return nil, ErrMismatch
		}
		for _, im := range doc.Images {
			if im.Block >= a.Blocks {
				return nil, ErrMismatch
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return x, nil
}

// VerifyArticle checks supplied article bytes, then their retained-HTML/profile and
// image associations. It does not fetch the HTML, raw source, recipe or figures,
// rerun extraction, certify a producer, or verify the semantic correctness of text.
// Quotes still use the SAME manifest-selected edition and saved location via
// htmledition.Quote. Returned article storage is independently owned.
func (m *Manifest) VerifyArticle(ctx context.Context, id string, slot int, body []byte, l htmlarticlejson.Limits) (htmlarticle.Article, error) {
	e, err := m.edition(ctx, id)
	if err != nil {
		return htmlarticle.Article{}, err
	}
	if slot < 0 || slot >= len(e.Documents) {
		return htmlarticle.Article{}, ErrInvalid
	}
	d := e.Documents[slot]
	// Admit bytes before hashing/decoding; the existing article reader checks its
	// other logical limits. Hashing is one bounded synchronous pass.
	if l.MaxEncodedBytes <= 0 {
		return htmlarticle.Article{}, ErrInvalid
	}
	if len(body) > l.MaxEncodedBytes || d.Article.Bytes > int64(l.MaxEncodedBytes) {
		return htmlarticle.Article{}, ErrLimit
	}
	if int64(len(body)) != d.Article.Bytes || sha256.Sum256(body) != d.Article.SHA256 {
		return htmlarticle.Article{}, ErrMismatch
	}
	a, err := htmlarticlejson.Decode(ctx, body, l)
	if err != nil {
		return htmlarticle.Article{}, err
	}
	if a.HTMLSHA256 != d.HTML.SHA256 || int64(a.HTMLBytes) != d.HTML.Bytes || a.Profile != d.Profile {
		return htmlarticle.Article{}, ErrMismatch
	}
	images := 0
	for bi, b := range a.Blocks {
		if err := ctx.Err(); err != nil {
			return htmlarticle.Article{}, err
		}
		if b.Kind != htmlarticle.Image {
			continue
		}
		if images >= len(d.Images) || d.Images[images].Block != bi || d.Images[images].Address != b.SourceURL {
			return htmlarticle.Article{}, ErrMismatch
		}
		images++
	}
	if images != len(d.Images) {
		return htmlarticle.Article{}, ErrMismatch
	}
	if err := ctx.Err(); err != nil {
		return htmlarticle.Article{}, err
	}
	return a, nil
}
