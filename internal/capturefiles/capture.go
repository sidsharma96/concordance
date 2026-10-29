// Package capturefiles reads a trusted prepared Git/HTML handoff from local files.
// It verifies bounded metadata and payload bytes, not build provenance, complete
// Git membership or authorization. The prepared importer owns those source checks.
package capturefiles

import (
	"context"
	"crypto/sha256"
	"errors"
	"io/fs"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/sidsharma96/concordance/internal/htmlarticle"
)

const Format = "concordance.git-html-capture/v1"
const DescriptorName = "capture.json"

var (
	ErrInvalid     = errors.New("invalid prepared capture files")
	ErrLimit       = errors.New("prepared capture file limit exceeded")
	ErrMismatch    = errors.New("prepared capture payload mismatch")
	ErrUnsupported = errors.New("unsupported prepared capture format")
)

// Description is an in-memory encoding/inspection value, not the wire schema.
// No source registration, credentials, collection, operation key or candidate ID
// is carried here. All aliases must remain unchanged while Encode uses them.
type Description struct{ Editions []Edition }
type Edition struct {
	ID       string
	Captures []Capture
}
type Capture struct {
	Source, Revision, Directory, Preparation, Toolchain string
	Recipe                                              File
	Dependencies                                        []string
	Profile                                             htmlarticle.Profile
	Pages                                               []Page
}
type Page struct {
	Path string
	HTML File
}

// File pins a regular payload beneath the capture root. Path is a transport name,
// not a Git path or a semantic import input. Digest/size never grant authority.
type File struct {
	Path   string
	SHA256 [sha256.Size]byte
	Bytes  int64
}

// Limits bound logical metadata and payload occurrences, not exact heap or CPU.
// Files counts references (one recipe per capture plus each page), even if shared.
// PayloadBytes also counts occurrences. Each unique path is read only once. Token
// allocation is bounded by DescriptorBytes before smaller string/count admission.
// Encode may allocate escaped output before checking DescriptorBytes, bounded by
// admitted values/counts. All fields are positive; a context deadline is required.
type Limits struct {
	MaxDescriptorBytes, MaxEditions, MaxCaptures, MaxPages, MaxDependencies, MaxFiles int
	MaxStringBytes, MaxValueBytes                                                     int
	MaxFileBytes, MaxPayloadBytes                                                     int64
}

func ready(ctx context.Context, l Limits) error {
	if ctx == nil {
		return ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, ok := ctx.Deadline(); !ok {
		return ErrInvalid
	}
	if l.MaxDescriptorBytes <= 0 || l.MaxDescriptorBytes == int(^uint(0)>>1) || l.MaxEditions <= 0 || l.MaxCaptures <= 0 || l.MaxPages <= 0 || l.MaxDependencies <= 0 || l.MaxFiles <= 0 || l.MaxStringBytes <= 0 || l.MaxValueBytes <= 0 || l.MaxFileBytes <= 0 || l.MaxPayloadBytes <= 0 || l.MaxFileBytes >= int64(^uint(0)>>1) {
		return ErrInvalid
	}
	return nil
}
func identifier(s string) bool {
	return s != "" && len(s) <= 256 && utf8.ValidString(s) && strings.IndexFunc(s, unicode.IsControl) < 0
}
func cleanPath(s string) bool {
	return gitPath(s) && !strings.ContainsRune(s, ':')
}
func gitPath(s string) bool {
	return s != "." && fs.ValidPath(s) && !strings.ContainsRune(s, '\\') && strings.IndexFunc(s, unicode.IsControl) < 0
}
func objectID(s string) bool {
	if len(s) != 40 && len(s) != 64 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
func take(n int, left *int) error {
	if n > *left {
		return ErrLimit
	}
	*left -= n
	return nil
}

type admission struct {
	l                                                      Limits
	values, editions, captures, pages, dependencies, files int
	payload                                                int64
}

func newAdmission(l Limits) admission {
	return admission{l: l, values: l.MaxValueBytes, editions: l.MaxEditions, captures: l.MaxCaptures, pages: l.MaxPages, dependencies: l.MaxDependencies, files: l.MaxFiles, payload: l.MaxPayloadBytes}
}
func (a *admission) text(s string) error {
	if len(s) > a.l.MaxStringBytes || len(s) > a.values {
		return ErrLimit
	}
	if !utf8.ValidString(s) || strings.IndexByte(s, 0) >= 0 {
		return ErrInvalid
	}
	a.values -= len(s)
	return nil
}
func (a *admission) file(f File) error {
	if err := take(1, &a.files); err != nil {
		return err
	}
	if err := a.text(f.Path); err != nil {
		return err
	}
	// The wire digest is always exactly 64 ASCII value bytes; no temporary
	// hexadecimal string is needed for this admission pass.
	if 64 > a.l.MaxStringBytes || 64 > a.values {
		return ErrLimit
	}
	a.values -= 64
	if !cleanPath(f.Path) || f.Path == DescriptorName || f.Bytes <= 0 {
		return ErrInvalid
	}
	if f.Bytes > a.l.MaxFileBytes || f.Bytes > a.payload {
		return ErrLimit
	}
	a.payload -= f.Bytes
	return nil
}

// validate admits all supplied metadata before building path/membership maps.
// Format-specific semantic processing and registered-source checks remain with
// preparedimport; this method does not certify any source inventory or renderer.
func validate(ctx context.Context, d Description, l Limits) ([]File, error) {
	a := newAdmission(l)
	if err := a.text(Format); err != nil {
		return nil, err
	}
	if err := take(len(d.Editions), &a.editions); err != nil {
		return nil, err
	}
	for _, e := range d.Editions {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := a.text(e.ID); err != nil {
			return nil, err
		}
		if !identifier(e.ID) {
			return nil, ErrInvalid
		}
		if err := take(len(e.Captures), &a.captures); err != nil {
			return nil, err
		}
		for _, c := range e.Captures {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			for _, s := range []string{c.Source, c.Revision, c.Directory, c.Preparation, c.Toolchain, c.Profile.ID, c.Profile.ArticleClass} {
				if err := a.text(s); err != nil {
					return nil, err
				}
			}
			if !identifier(c.Source) || !objectID(c.Revision) || c.Directory != "." && !gitPath(c.Directory) || c.Preparation == "" || c.Toolchain == "" || c.Profile.ID == "" || c.Profile.ArticleClass == "" {
				return nil, ErrInvalid
			}
			for _, ss := range [][]string{c.Profile.OmitClasses, c.Profile.OmitIDs, c.Profile.NoticeClasses} {
				if len(ss) > 32 {
					return nil, ErrLimit
				}
				for _, s := range ss {
					if err := a.text(s); err != nil {
						return nil, err
					}
				}
			}
			if err := a.file(c.Recipe); err != nil {
				return nil, err
			}
			if err := take(len(c.Dependencies), &a.dependencies); err != nil {
				return nil, err
			}
			for _, s := range c.Dependencies {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
				if err := a.text(s); err != nil {
					return nil, err
				}
				if !gitPath(s) {
					return nil, ErrInvalid
				}
			}
			if err := take(len(c.Pages), &a.pages); err != nil {
				return nil, err
			}
			for _, p := range c.Pages {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
				if err := a.text(p.Path); err != nil {
					return nil, err
				}
				if !gitPath(p.Path) || !strings.HasSuffix(p.Path, ".md") || c.Directory != "." && !strings.HasPrefix(p.Path, c.Directory+"/") {
					return nil, ErrInvalid
				}
				if err := a.file(p.HTML); err != nil {
					return nil, err
				}
			}
		}
	}
	editions := map[string]bool{}
	byPath := map[string]File{}
	files := []File{}
	put := func(f File) error {
		if prev, ok := byPath[f.Path]; ok {
			if prev != f {
				return ErrMismatch
			}
			return nil
		}
		byPath[f.Path] = f
		files = append(files, f)
		return nil
	}
	for _, e := range d.Editions {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if editions[e.ID] {
			return nil, ErrInvalid
		}
		editions[e.ID] = true
		sources := map[string]bool{}
		for _, c := range e.Captures {
			if sources[c.Source] {
				return nil, ErrInvalid
			}
			sources[c.Source] = true
			if err := put(c.Recipe); err != nil {
				return nil, err
			}
			paths := map[string]bool{}
			for _, p := range c.Pages {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
				if paths[p.Path] {
					return nil, ErrInvalid
				}
				paths[p.Path] = true
				if err := put(p.HTML); err != nil {
					return nil, err
				}
			}
			for _, p := range c.Dependencies {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
				if paths[p] {
					return nil, ErrInvalid
				}
				paths[p] = true
			}
		}
	}
	return files, ctx.Err()
}
