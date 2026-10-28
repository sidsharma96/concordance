package htmledition

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"unicode/utf8"

	"github.com/sidsharma96/concordance/internal/htmlpassage"
	"github.com/sidsharma96/concordance/internal/termsearch"
)

const magic = "CNDX\x00"
const wireVersion = 1

type encoder struct {
	ctx  context.Context
	max  int
	body []byte
	err  error
}

func (w *encoder) raw(b []byte) {
	if w.err != nil {
		return
	}
	if err := w.ctx.Err(); err != nil {
		w.err = err
		return
	}
	if len(b) > w.max-len(w.body) {
		w.err = ErrLimit
		return
	}
	w.body = append(w.body, b...)
}
func (w *encoder) num(n int) {
	if n < 0 {
		w.err = ErrInvalid
		return
	}
	var scratch [binary.MaxVarintLen64]byte
	k := binary.PutUvarint(scratch[:], uint64(n))
	w.raw(scratch[:k])
}
func (w *encoder) str(s string) { w.num(len(s)); w.raw([]byte(s)) }

// Encode returns canonical owned bytes, or nil on error. ExportEdition admits
// the selected index under these limits before copying counts. Maps are sorted;
// stored output never depends on map iteration or native Go struct layout.
func Encode(ctx context.Context, e *Edition, l Limits) ([]byte, error) {
	if err := ready(ctx, l); err != nil {
		return nil, err
	}
	if !e.valid() {
		return nil, ErrInvalid
	}
	if err := validateLocations(ctx, e, l); err != nil {
		return nil, err
	}
	c, found, err := e.index.ExportEdition(ctx, e.id, countLimits(l))
	if err != nil {
		return nil, countError(err)
	}
	if !found || len(c.Lengths) != len(e.locations) {
		return nil, ErrInvalid
	}
	rows := 0
	for _, w := range c.Words {
		if len(w.Term) > l.MaxTermBytes {
			return nil, ErrLimit
		}
		rows += len(w.Rows)
	} // export admitted the sum under MaxPostings.
	w := encoder{ctx: ctx, max: l.MaxEncodedBytes}
	w.raw([]byte(magic))
	w.num(wireVersion)
	for _, s := range []string{e.id, articleFormat, passageProcessor, fieldProcessor, termsVersion} {
		w.str(s)
	}
	for _, n := range []int{len(e.articles), len(e.locations), len(c.Words), rows, c.TotalTerms} {
		w.num(n)
	}
	for _, a := range e.articles {
		w.raw(a.Object.SHA256[:])
		w.num(a.Object.Bytes)
		w.num(a.Blocks)
		w.num(a.Passages)
	}
	for i, r := range e.locations {
		w.num(r.Article)
		w.num(r.Body.First)
		w.num(r.Body.After)
		w.num(c.Lengths[i])
	}
	for _, word := range c.Words {
		w.str(word.Term)
		w.num(len(word.Rows))
		for _, r := range word.Rows {
			w.num(r.Passage)
			w.num(r.Body)
			w.num(r.Headings)
			w.num(r.Labels)
		}
	}
	if w.err != nil {
		return nil, w.err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return w.body, nil
}

type decoder struct {
	ctx      context.Context
	body     []byte
	position int
	err      error
}

func (r *decoder) num() int {
	if r.err != nil {
		return 0
	}
	if err := r.ctx.Err(); err != nil {
		r.err = err
		return 0
	}
	n, k := binary.Uvarint(r.body[r.position:])
	if k <= 0 || n > uint64(int(^uint(0)>>1)) || k > 1 && r.body[r.position+k-1] == 0 {
		r.err = ErrInvalid
		return 0
	}
	r.position += k
	return int(n)
}
func (r *decoder) raw(n int) []byte {
	if r.err != nil {
		return nil
	}
	if n < 0 || n > len(r.body)-r.position {
		r.err = ErrInvalid
		return nil
	}
	b := r.body[r.position : r.position+n]
	r.position += n
	return b
}
func (r *decoder) str(maxBytes int) string {
	n := r.num()
	if r.err != nil {
		return ""
	}
	if n <= 0 {
		r.err = ErrInvalid
		return ""
	}
	if n > maxBytes {
		r.err = ErrLimit
		return ""
	}
	b := r.raw(n)
	if r.err != nil {
		return ""
	}
	if !utf8.Valid(b) || bytes.IndexByte(b, 0) >= 0 {
		r.err = ErrInvalid
		return ""
	}
	return string(b)
}

// Open checks the actual file against the independent expected reference BEFORE
// parsing. Callers supply an authorized generation's reference/edition; this method
// cannot establish its provenance. It validates all records before exposing state,
// retaining no input byte/string aliases. No article body is read and no passage
// builder runs here. Unknown versions and trailing bytes fail closed.
func Open(ctx context.Context, body []byte, expected Ref, id string, l Limits) (*Edition, error) {
	if err := ready(ctx, l); err != nil {
		return nil, err
	}
	if !validString(id) || expected.Bytes <= 0 {
		return nil, ErrInvalid
	}
	if len(body) > l.MaxEncodedBytes || expected.Bytes > l.MaxEncodedBytes {
		return nil, ErrLimit
	}
	if len(body) != expected.Bytes || sha256.Sum256(body) != expected.SHA256 {
		return nil, ErrMismatch
	}
	if !bytes.HasPrefix(body, []byte(magic)) {
		return nil, ErrUnsupported
	}
	r := decoder{ctx: ctx, body: body, position: len(magic)}
	v := r.num()
	if r.err != nil {
		return nil, r.err
	}
	if v != wireVersion {
		return nil, ErrUnsupported
	}
	edition := r.str(min(l.MaxKeyBytes, l.MaxPreparedBytes))
	if r.err != nil {
		return nil, r.err
	}
	if edition != id {
		return nil, ErrMismatch
	}
	for _, want := range []string{articleFormat, passageProcessor, fieldProcessor, termsVersion} {
		got := r.str(128)
		if r.err != nil {
			return nil, r.err
		}
		if got != want {
			return nil, ErrUnsupported
		}
	}
	a, p, vocabulary, postings, total := r.num(), r.num(), r.num(), r.num(), r.num()
	if r.err != nil {
		return nil, r.err
	}
	if a > l.MaxArticles || p > l.MaxPassages || vocabulary > l.MaxVocabulary || postings > l.MaxPostings || total > l.MaxTerms {
		return nil, ErrLimit
	}
	if vocabulary > postings {
		return nil, ErrInvalid
	}
	// Minimum record bytes guard against enormous declarations in tiny files even
	// when the caller chooses large logical caps. Subtraction avoids overflow.
	remaining := len(body) - r.position
	for _, pair := range [][2]int{{a, 35}, {p, 4}, {vocabulary, 3}, {postings, 4}} {
		if pair[0] > remaining/pair[1] {
			return nil, ErrInvalid
		}
		remaining -= pair[0] * pair[1]
	}
	e := &Edition{id: edition, articles: make([]ArticleRef, a), locations: make([]Location, p)}
	for i := range e.articles {
		var digest [sha256.Size]byte
		copy(digest[:], r.raw(sha256.Size))
		e.articles[i] = ArticleRef{Ref{digest, r.num()}, r.num(), r.num()}
	}
	c := termsearch.EditionCounts{TermsVersion: termsVersion, Edition: edition, Lengths: make([]int, p), TotalTerms: total, Words: make([]termsearch.CountedWord, 0, vocabulary)}
	for i := range e.locations {
		e.locations[i] = Location{r.num(), htmlpassage.Range{First: r.num(), After: r.num()}}
		c.Lengths[i] = r.num()
	}
	left := postings
	for range vocabulary {
		word := r.str(l.MaxTermBytes)
		n := r.num()
		if r.err != nil {
			return nil, r.err
		}
		if n <= 0 || n > left || n > (len(body)-r.position)/4 {
			return nil, ErrInvalid
		}
		left -= n
		rows := make([]termsearch.FieldCount, n)
		for i := range rows {
			rows[i] = termsearch.FieldCount{Passage: r.num(), Body: r.num(), Headings: r.num(), Labels: r.num()}
		}
		if r.err != nil {
			return nil, r.err
		}
		c.Words = append(c.Words, termsearch.CountedWord{Term: word, Rows: rows})
	}
	if r.err != nil {
		return nil, r.err
	}
	if left != 0 || r.position != len(body) {
		return nil, ErrInvalid
	}
	if err := validateLocations(ctx, e, l); err != nil {
		return nil, err
	}
	index, err := termsearch.NewCountIndex(ctx, c, countLimits(l))
	if err != nil {
		return nil, countError(err)
	}
	e.index = index
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return e, nil
}
