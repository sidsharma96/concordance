package generationmanifest

import (
	"context"
	"encoding/hex"
	"path"
	"strings"
	"unicode"
	"unicode/utf8"
)

type admission struct {
	l                                          Limits
	values, captures, files, documents, images int
	bytes                                      int64
	sizes                                      map[[32]byte]int64
	objects                                    []Ref
	filesBySource, docsBySource                map[string]int
}

func newAdmission(l Limits) admission {
	return admission{l: l, values: l.MaxValueBytes, captures: l.MaxCaptures, files: l.MaxFiles, documents: l.MaxDocuments, images: l.MaxImages, bytes: l.MaxReferencedBytes,
		sizes: make(map[[32]byte]int64), filesBySource: make(map[string]int), docsBySource: make(map[string]int)}
}
func (a *admission) text(s string) error {
	if len(s) > a.l.MaxStringBytes || len(s) > a.values {
		return ErrLimit
	}
	if !utf8.ValidString(s) || strings.ContainsRune(s, 0) {
		return ErrInvalid
	}
	a.values -= len(s)
	return nil
}
func (a *admission) texts(ss ...string) error {
	for _, s := range ss {
		if err := a.text(s); err != nil {
			return err
		}
	}
	return nil
}
func (a *admission) ref(r Ref, empty bool) error {
	if r.Bytes < 0 || !empty && r.Bytes == 0 {
		return ErrInvalid
	}
	if r.Bytes > a.l.MaxObjectBytes || r.Bytes > a.bytes {
		return ErrLimit
	}
	a.bytes -= r.Bytes
	// Encoded digests are value strings too, even when the object is shared.
	if err := a.text(hex.EncodeToString(r.SHA256[:])); err != nil {
		return err
	}
	if size, exists := a.sizes[r.SHA256]; exists {
		if size != r.Bytes {
			return ErrInvalid
		}
	} else {
		a.sizes[r.SHA256] = r.Bytes
		a.objects = append(a.objects, r)
	}
	return nil
}
func take(n int, left *int) error {
	if n < 0 || n > *left {
		return ErrLimit
	}
	*left -= n
	return nil
}
func identifier(s string) bool {
	return s != "" && len(s) <= 256 && utf8.ValidString(s) && strings.IndexFunc(s, unicode.IsControl) < 0
}
func cleanPath(s string, directory bool) bool {
	return s != "" && (directory || s != ".") && !strings.HasPrefix(s, "/") && s != ".." && !strings.HasPrefix(s, "../") && !strings.Contains(s, "\\") && strings.IndexFunc(s, unicode.IsControl) < 0 && path.Clean(s) == s
}
func gitID(s string, width int) bool {
	if (len(s) != 40 && len(s) != 64) || width != 0 && len(s) != width {
		return false
	}
	for _, c := range []byte(s) {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
func imagePath(s string) (string, bool) {
	if !strings.HasPrefix(s, "/images/") || s == "/images/" || strings.ContainsAny(s, "\\%?#:") || strings.IndexFunc(s, unicode.IsControl) >= 0 || path.Clean(s) != s {
		return "", false
	}
	return "static" + s, true
}
func profile(s string) bool {
	if len(s) == 0 || len(s) > 128 {
		return false
	}
	for _, b := range []byte(s) {
		if !(b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || strings.ContainsRune("_/.-", rune(b))) {
			return false
		}
	}
	return true
}

func validate(ctx context.Context, d Description, l Limits) ([]Ref, error) {
	if !identifier(d.Collection) || !identifier(d.Generation) {
		return nil, ErrInvalid
	}
	if len(d.Sources) > l.MaxSources || len(d.Editions) > l.MaxEditions {
		return nil, ErrLimit
	}
	a := newAdmission(l)
	if err := a.texts(Format, d.Collection, d.Generation, articleFormat, indexFormat); err != nil {
		return nil, err
	}
	sources := make(map[string]bool, len(d.Sources))
	for _, s := range d.Sources {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := a.texts(s.ID, s.Repository); err != nil {
			return nil, err
		}
		if !identifier(s.ID) || s.Repository == "" || sources[s.ID] {
			return nil, ErrInvalid
		}
		sources[s.ID] = true
	}
	editions := make(map[string]bool, len(d.Editions))
	for _, e := range d.Editions {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := a.text(e.ID); err != nil {
			return nil, err
		}
		if !identifier(e.ID) || editions[e.ID] {
			return nil, ErrInvalid
		}
		editions[e.ID] = true
		if err := a.ref(e.Index, false); err != nil {
			return nil, err
		}
		if err := take(len(e.Captures), &a.captures); err != nil {
			return nil, err
		}
		if err := take(len(e.Documents), &a.documents); err != nil {
			return nil, err
		}
		captured := make(map[string]bool, len(e.Captures))
		for _, c := range e.Captures {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if err := a.texts(c.Source, c.Revision, c.Directory, c.Preparation, c.Toolchain); err != nil {
				return nil, err
			}
			if !sources[c.Source] || captured[c.Source] || !gitID(c.Revision, 0) || !cleanPath(c.Directory, true) || c.Preparation == "" || c.Toolchain == "" {
				return nil, ErrInvalid
			}
			captured[c.Source] = true
			if err := a.ref(c.Recipe, true); err != nil {
				return nil, err
			}
			if err := take(len(c.Files), &a.files); err != nil {
				return nil, err
			}
			if len(c.Files) > l.MaxSourceFiles-a.filesBySource[c.Source] {
				return nil, ErrLimit
			}
			a.filesBySource[c.Source] += len(c.Files)
			paths := make(map[string]bool, len(c.Files))
			for _, f := range c.Files {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
				if err := a.texts(f.Path, f.BlobID); err != nil {
					return nil, err
				}
				if !cleanPath(f.Path, false) || !gitID(f.BlobID, len(c.Revision)) || paths[f.Path] {
					return nil, ErrInvalid
				}
				paths[f.Path] = true
				if err := a.ref(f.Object, true); err != nil {
					return nil, err
				}
			}
		}
		type key struct{ source, native string }
		keys := make(map[key]bool, len(e.Documents))
		for _, doc := range e.Documents {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if err := a.texts(doc.NativeID, doc.Location, doc.Profile); err != nil {
				return nil, err
			}
			if doc.Capture < 0 || doc.Capture >= len(e.Captures) {
				return nil, ErrInvalid
			}
			c := e.Captures[doc.Capture]
			if doc.File < 0 || doc.File >= len(c.Files) {
				return nil, ErrInvalid
			}
			f := c.Files[doc.File]
			if doc.NativeID != f.Path || doc.Location != f.Path || !profile(doc.Profile) || !strings.HasSuffix(f.Path, ".md") || c.Directory != "." && !strings.HasPrefix(f.Path, c.Directory+"/") {
				return nil, ErrInvalid
			}
			k := key{c.Source, doc.NativeID}
			if keys[k] {
				return nil, ErrInvalid
			}
			keys[k] = true
			if a.docsBySource[c.Source] >= l.MaxSourceDocuments {
				return nil, ErrLimit
			}
			a.docsBySource[c.Source]++
			if err := a.ref(doc.HTML, false); err != nil {
				return nil, err
			}
			if err := a.ref(doc.Article, false); err != nil {
				return nil, err
			}
			if err := take(len(doc.Images), &a.images); err != nil {
				return nil, err
			}
			last := -1
			for _, im := range doc.Images {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
				if err := a.text(im.Address); err != nil {
					return nil, err
				}
				name, ok := imagePath(im.Address)
				if !ok {
					return nil, ErrUnsupported
				}
				if im.Block <= last || im.File < 0 || im.File >= len(c.Files) || c.Files[im.File].Path != name {
					return nil, ErrInvalid
				}
				last = im.Block
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return a.objects, nil
}
