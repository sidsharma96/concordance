package generationmanifest

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"hash"
	"io"
	"slices"
	"sort"

	"github.com/sidsharma96/concordance/internal/htmlarticle"
)

// InputFingerprintVersion identifies the framed input-comparison specification,
// independently of the manifest's output encoding. Retain old comparison rules
// for outstanding saved keys when this specification changes.
const InputFingerprintVersion = "concordance.git-html-input/v1"

// InputRules describes the trusted producer's actual processing rules, not an
// attestation that they ran. Versions are explicit; only the current retained
// Git/HTML pipeline is supported. Profiles must exactly cover document profile IDs.
// Preserve every alias unchanged through InputFingerprint or PublishCaptured.
type InputRules struct {
	ArticleProcessor, PassageProcessor, FieldProcessor, TermsVersion string
	Profiles                                                         []htmlarticle.Profile
}

// InputFingerprint compares captured Git/HTML inputs and rules, not generated
// results. It includes actual HTML/file/recipe refs, whose bytes and provenance
// must be established by the trusted owner. This metadata-only method does no I/O,
// source capture, completeness check, authorization or body verification.
//
// Source/edition/capture/file order and profile membership-list order are normalized;
// document slot order remains significant. Candidate generation, article/index
// refs and derived image block ordinals are excluded. Full artifact preflight must
// precede use in publication. Never expose this as a public caller-digest API.
//
// maxBytes bounds the entire framed hash preimage, including prefixes. Count it
// before allocating ordering scratch or hashing. Manifest metadata was admitted
// when opened. Sorting is nonpreemptive within those bounds; traversal checks the
// deadline context cooperatively. Work is O(metadata bytes + sum(n log n)); scratch
// is O(metadata ordering + profile IDs), not body bytes. No aliases survive return.
// Errors return a zero digest. Concurrent calls are safe with unchanged rules.
func (m *Manifest) InputFingerprint(ctx context.Context, r InputRules, maxBytes int) ([32]byte, error) {
	if err := contextReady(ctx); err != nil {
		return [32]byte{}, err
	}
	if !m.valid() || maxBytes <= 0 {
		return [32]byte{}, ErrInvalid
	}
	if r.ArticleProcessor != "htmlarticle/v1" || r.PassageProcessor != "htmlpassage/v1" || r.FieldProcessor != "htmlterms/v1" || r.TermsVersion != "technical/v1/unicode-17.0.0" {
		return [32]byte{}, ErrUnsupported
	}
	count := inputWriter{ctx: ctx, left: maxBytes}
	m.writeInput(&count, r, false)
	if count.err != nil {
		return [32]byte{}, count.err
	}
	if err := m.validateInputRules(ctx, r); err != nil {
		return [32]byte{}, err
	}
	w := inputWriter{ctx: ctx, left: maxBytes, h: sha256.New()}
	m.writeInput(&w, r, true)
	if w.err != nil {
		return [32]byte{}, w.err
	}
	if err := ctx.Err(); err != nil {
		return [32]byte{}, err
	}
	var digest [32]byte
	copy(digest[:], w.h.Sum(nil))
	return digest, nil
}

type inputWriter struct {
	ctx  context.Context
	left int
	h    hash.Hash
	err  error
}

func (w *inputWriter) admit(n int) bool {
	if w.err != nil {
		return false
	}
	if w.err = w.ctx.Err(); w.err != nil {
		return false
	}
	if n > w.left {
		w.err = ErrLimit
		return false
	}
	w.left -= n
	return true
}
func (w *inputWriter) number(n uint64) {
	if !w.admit(8) {
		return
	}
	if w.h != nil {
		var b [8]byte
		binary.BigEndian.PutUint64(b[:], n)
		_, _ = w.h.Write(b[:])
	}
}
func (w *inputWriter) text(s string) {
	w.number(uint64(len(s)))
	if !w.admit(len(s)) {
		return
	}
	// The only writer is SHA-256: these in-memory writes never return an error.
	if w.h != nil {
		_, _ = io.WriteString(w.h, s)
	}
}
func (w *inputWriter) ref(r Ref) {
	if !w.admit(len(r.SHA256)) {
		return
	}
	if w.h != nil {
		_, _ = w.h.Write(r.SHA256[:])
	}
	w.number(uint64(r.Bytes))
}
func (w *inputWriter) each(n int, canonical bool, less func(int, int) bool, visit func(int)) {
	if w.err != nil {
		return
	}
	if !canonical {
		for i := 0; i < n && w.err == nil; i++ {
			visit(i)
		}
		return
	}
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(i, j int) bool { return less(order[i], order[j]) })
	for _, i := range order {
		if w.err != nil {
			return
		}
		visit(i)
	}
}
func (w *inputWriter) tokens(ss []string, canonical bool) {
	w.number(uint64(len(ss)))
	w.each(len(ss), canonical, func(i, j int) bool { return ss[i] < ss[j] }, func(i int) { w.text(ss[i]) })
}
func (m *Manifest) writeInput(w *inputWriter, r InputRules, canonical bool) {
	d := &m.description
	w.text(InputFingerprintVersion)
	w.text(d.Collection)
	w.text(articleFormat)
	w.text(indexFormat)
	w.text(r.ArticleProcessor)
	w.text(r.PassageProcessor)
	w.text(r.FieldProcessor)
	w.text(r.TermsVersion)
	w.number(uint64(len(r.Profiles)))
	w.each(len(r.Profiles), canonical, func(i, j int) bool { return r.Profiles[i].ID < r.Profiles[j].ID }, func(i int) {
		p := r.Profiles[i]
		w.text(p.ID)
		w.text(p.ArticleClass)
		w.tokens(p.OmitClasses, canonical)
		w.tokens(p.OmitIDs, canonical)
		w.tokens(p.NoticeClasses, canonical)
	})
	w.number(uint64(len(d.Sources)))
	w.each(len(d.Sources), canonical, func(i, j int) bool { return d.Sources[i].ID < d.Sources[j].ID }, func(i int) { w.text(d.Sources[i].ID); w.text(d.Sources[i].Repository) })
	w.number(uint64(len(d.Editions)))
	w.each(len(d.Editions), canonical, func(i, j int) bool { return d.Editions[i].ID < d.Editions[j].ID }, func(i int) {
		e := &d.Editions[i]
		w.text(e.ID)
		w.number(uint64(len(e.Captures)))
		w.each(len(e.Captures), canonical, func(i, j int) bool { return e.Captures[i].Source < e.Captures[j].Source }, func(i int) {
			c := &e.Captures[i]
			w.text(c.Source)
			w.text(c.Revision)
			w.text(c.Directory)
			w.text(c.Preparation)
			w.text(c.Toolchain)
			w.ref(c.Recipe)
			w.number(uint64(len(c.Files)))
			w.each(len(c.Files), canonical, func(i, j int) bool { return c.Files[i].Path < c.Files[j].Path }, func(i int) { f := c.Files[i]; w.text(f.Path); w.text(f.BlobID); w.ref(f.Object) })
		})
		w.number(uint64(len(e.Documents)))
		// Slots remain in supplied order: stable ranking ties observe that order.
		for _, doc := range e.Documents {
			if w.err != nil {
				return
			}
			c := &e.Captures[doc.Capture]
			w.text(c.Source)
			w.text(c.Files[doc.File].Path)
			w.text(doc.NativeID)
			w.text(doc.Location)
			w.text(doc.Profile)
			w.ref(doc.HTML)
		}
	})
}
func (m *Manifest) validateInputRules(ctx context.Context, r InputRules) error {
	profiles := make(map[string]bool, len(r.Profiles))
	for _, p := range r.Profiles {
		if err := ctx.Err(); err != nil {
			return err
		}
		_, duplicate := profiles[p.ID]
		if !profile(p.ID) || !inputToken(p.ArticleClass) || duplicate {
			return ErrInvalid
		}
		for _, ss := range [][]string{p.OmitClasses, p.OmitIDs, p.NoticeClasses} {
			if len(ss) > 32 {
				return ErrInvalid
			}
			for i, s := range ss {
				if !inputToken(s) || slices.Contains(ss[:i], s) {
					return ErrInvalid
				}
			}
		}
		if slices.Contains(p.OmitClasses, p.ArticleClass) {
			return ErrInvalid
		}
		profiles[p.ID] = false
	}
	for _, e := range m.description.Editions {
		for _, doc := range e.Documents {
			if err := ctx.Err(); err != nil {
				return err
			}
			if _, ok := profiles[doc.Profile]; !ok {
				return ErrInvalid
			}
			profiles[doc.Profile] = true
		}
	}
	for _, used := range profiles {
		if !used {
			return ErrInvalid
		}
	}
	return ctx.Err()
}
func inputToken(s string) bool {
	if len(s) == 0 || len(s) > 128 {
		return false
	}
	for _, c := range []byte(s) {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}
