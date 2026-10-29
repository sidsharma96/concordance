// Package generationmanifest binds retained Git/HTML artifacts to logical documents
// in one bounded generation. It verifies metadata and selected artifact bindings,
// not source completeness, build provenance, authorization or publication.
package generationmanifest

import (
	"context"
	"crypto/sha256"
	"errors"
)

const Format = "concordance.git-html-generation/v1"

// Literal compatibility pins belong to this retained format, not the latest writer.
const articleFormat = "concordance.article/v1"
const indexFormat = "concordance.html-edition/v1"

var (
	ErrInvalid     = errors.New("invalid generation manifest")
	ErrLimit       = errors.New("generation manifest limit exceeded")
	ErrUnsupported = errors.New("unsupported generation representation")
	ErrMismatch    = errors.New("generation artifact binding mismatch")
	ErrNotFound    = errors.New("edition not declared in generation")
)

// Ref pins bytes within the caller's authorized collection; it grants no access.
type Ref struct {
	SHA256 [sha256.Size]byte
	Bytes  int64
}

// Description is an input/diagnostic value, not the persisted wire schema. Encode
// borrows all aliases until return. Describe returns independently mutable slices.
// Sources, editions, captures, files and document slots retain their supplied order.
type Description struct {
	Collection, Generation string
	Sources                []Source
	Editions               []Edition
}
type Source struct{ ID, Repository string }
type Edition struct {
	ID        string
	Index     Ref
	Captures  []Capture
	Documents []Document
}

// Capture records one pinned Git source/edition selection. Files include raw
// Markdown and explicit dependencies. Recipe is retained configuration/description
// bytes, never executed here. Preparation/Toolchain identify the trusted producer's
// rules; labels and hashes do not establish that it ran or captured complete inputs.
type Capture struct {
	Source, Revision, Directory, Preparation, Toolchain string
	Recipe                                              Ref
	Files                                               []File
}
type File struct {
	Path, BlobID string
	Object       Ref
}

// Document's Capture/File are edition/capture-local ordinals. NativeID and Location
// both equal the full source path under this initial Git identity profile. HTML is
// retained input; Article is derived article JSON. Profile pins extraction metadata.
type Document struct {
	Capture, File               int
	NativeID, Location, Profile string
	HTML, Article               Ref
	Images                      []Image
}

// Image connects one retained article block/address to an original captured file.
// This v1 mapping supports /images/... -> static/images/... only. No interpretation,
// safe embedding policy or permission is implied by retaining SVG/other image bytes.
type Image struct {
	Block   int
	Address string
	File    int
}

// Limits admit logical counts/bytes, not exact heap or CPU quotas. Reference bytes
// count every declared reference occurrence, including shared bytes across captures
// or slots; image ordinals add no second reference. All fields must be positive.
// A decoder token/encoded record may allocate before smaller value/output admission,
// bounded by encoded input or admitted record strings respectively.
// Global list counts apply before append. Per-source quotas and cross-references
// are checked after that bounded decode, before any state is exposed.
type Limits struct {
	MaxEncodedBytes, MaxSources, MaxEditions, MaxCaptures, MaxFiles int
	MaxDocuments, MaxImages, MaxSourceFiles, MaxSourceDocuments     int
	MaxStringBytes, MaxValueBytes                                   int
	MaxObjectBytes, MaxReferencedBytes                              int64
}

// Manifest owns immutable private metadata; copies of the pointer may be shared
// by concurrent readers. No I/O, goroutines or Close. Encoded input is not retained.
type Manifest struct {
	description Description
	objects     []Ref
}

func (m *Manifest) valid() bool {
	return m != nil && m.description.Collection != "" && m.description.Generation != ""
}

func contextReady(ctx context.Context) error {
	if ctx == nil {
		return ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, ok := ctx.Deadline(); !ok {
		return ErrInvalid
	}
	return nil
}
func ready(ctx context.Context, l Limits) error {
	if err := contextReady(ctx); err != nil {
		return err
	}
	if l.MaxEncodedBytes <= 0 || l.MaxSources <= 0 || l.MaxEditions <= 0 || l.MaxCaptures <= 0 || l.MaxFiles <= 0 || l.MaxDocuments <= 0 || l.MaxImages <= 0 || l.MaxSourceFiles <= 0 || l.MaxSourceDocuments <= 0 || l.MaxStringBytes <= 0 || l.MaxValueBytes <= 0 || l.MaxObjectBytes <= 0 || l.MaxReferencedBytes <= 0 {
		return ErrInvalid
	}
	return nil
}

// Objects returns owned, de-duplicated (SHA256,Bytes) refs in first-reference order,
// excluding the manifest itself. It is a list to confirm, not existence evidence.
func (m *Manifest) Objects(ctx context.Context) ([]Ref, error) {
	if err := contextReady(ctx); err != nil {
		return nil, err
	}
	if !m.valid() {
		return nil, ErrInvalid
	}
	out := make([]Ref, len(m.objects))
	for i, r := range m.objects {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		out[i] = r
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// Describe copies all slice storage. Strings are immutable values and may share
// internal bytes. This O(metadata) diagnostic copy is not a paged metadata reader.
func (m *Manifest) Describe(ctx context.Context) (Description, error) {
	if err := contextReady(ctx); err != nil {
		return Description{}, err
	}
	if !m.valid() {
		return Description{}, ErrInvalid
	}
	d := m.description
	d.Sources = append([]Source{}, d.Sources...)
	d.Editions = append([]Edition{}, d.Editions...)
	for ei := range d.Editions {
		if err := ctx.Err(); err != nil {
			return Description{}, err
		}
		e := &d.Editions[ei]
		e.Captures = append([]Capture{}, e.Captures...)
		for ci := range e.Captures {
			if err := ctx.Err(); err != nil {
				return Description{}, err
			}
			c := &e.Captures[ci]
			c.Files = append([]File{}, c.Files...)
		}
		e.Documents = append([]Document{}, e.Documents...)
		for di := range e.Documents {
			if err := ctx.Err(); err != nil {
				return Description{}, err
			}
			doc := &e.Documents[di]
			doc.Images = append([]Image{}, doc.Images...)
		}
	}
	if err := ctx.Err(); err != nil {
		return Description{}, err
	}
	return d, nil
}
