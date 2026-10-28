package termsearch

import (
	"context"
	"slices"
	"strings"
)

// EditionCounts is an owned, mutable transport of one edition, not a wire format,
// provenance or authorization. Lengths include all three fields and empty passages.
// Words and each word's rows must be strictly increasing by key and passage position.
// ExportEdition returns independent storage; NewCountIndex borrows unchanged until
// return and retains no aliases. Neither operation expands counts into ordered terms.
type EditionCounts struct {
	TermsVersion string
	Edition      string
	Lengths      []int
	TotalTerms   int
	Words        []CountedWord
}
type CountedWord struct {
	Term string
	Rows []FieldCount
}
type FieldCount struct {
	Passage  int
	Body     int
	Headings int
	Labels   int
}

// ExportEdition returns an independent count copy of the selected edition. Missing
// editions return found=false and zero output. Work includes sorting V word keys
// (O(V log V) comparisons) and copying P lengths, U rows and keys. Temporary output
// can be as large as that edition's admitted index. Limits admit the selected
// edition before table allocation; input bytes charge every counted occurrence.
// No I/O or goroutines start.
func (x *Index) ExportEdition(ctx context.Context, id string, l BuildLimits) (EditionCounts, bool, error) {
	fail := func(err error) (EditionCounts, bool, error) { return EditionCounts{}, false, err }
	if err := indexContext(ctx); err != nil {
		return fail(err)
	}
	if x == nil || x.data == nil || !validString(id) || !validBuildLimits(l) {
		return fail(ErrInvalid)
	}
	e := x.data.editions[id]
	if e == nil {
		return EditionCounts{}, false, nil
	}
	if len(e.lengths) > l.MaxPassages || len(e.terms) > l.MaxVocabulary || e.totalLength > l.MaxTerms {
		return fail(ErrLimit)
	}
	bytesLeft, keysLeft, rowsLeft := l.MaxInputBytes, l.MaxKeyBytes, l.MaxPostings
	for _, s := range []string{x.data.version, id} {
		if len(s) > bytesLeft || len(s) > keysLeft {
			return fail(ErrLimit)
		}
		bytesLeft -= len(s)
		keysLeft -= len(s)
	}
	// Admit this selected edition before allocating the exported tables.
	for key, list := range e.terms {
		if err := ctx.Err(); err != nil {
			return fail(err)
		}
		if len(key) > keysLeft || len(list.rows) > rowsLeft {
			return fail(ErrLimit)
		}
		keysLeft -= len(key)
		rowsLeft -= len(list.rows)
		for _, row := range list.rows {
			if err := ctx.Err(); err != nil {
				return fail(err)
			}
			n := row.frequency()
			if n > bytesLeft/len(key) {
				return fail(ErrLimit)
			}
			bytesLeft -= n * len(key)
		}
	}
	keys := make([]string, 0, len(e.terms))
	for key := range e.terms {
		if err := ctx.Err(); err != nil {
			return fail(err)
		}
		keys = append(keys, key)
	}
	slices.Sort(keys)
	out := EditionCounts{TermsVersion: strings.Clone(x.data.version), Edition: strings.Clone(id), Lengths: make([]int, len(e.lengths)), TotalTerms: e.totalLength, Words: make([]CountedWord, 0, len(keys))}
	for i, n := range e.lengths {
		if err := ctx.Err(); err != nil {
			return fail(err)
		}
		out.Lengths[i] = n
	}
	for _, key := range keys {
		if err := ctx.Err(); err != nil {
			return fail(err)
		}
		list := e.terms[key]
		w := CountedWord{Term: strings.Clone(key), Rows: make([]FieldCount, len(list.rows))}
		for i, r := range list.rows {
			if err := ctx.Err(); err != nil {
				return fail(err)
			}
			w.Rows[i] = FieldCount{r.passage, r.body, r.headings, r.labels}
		}
		out.Words = append(out.Words, w)
	}
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	return out, true, nil
}

// NewCountIndex validates a complete single-edition count transport, independently
// recomputing lengths, statistics and occurrence-weighted logical input bytes. It
// uses checked budgets before addition/multiplication; inconsistent totals fail.
// It then clones keys and rows into immutable state. Zero-length passages are valid.
// Expected work O(P+V+U+key bytes) under ordinary map assumptions; owned state is
// O(P+V+U+key bytes), plus validation scratch O(P). Limits are logical, not exact heap.
// Query admission describes this one edition, not an unseen generation's other data.
func NewCountIndex(ctx context.Context, c EditionCounts, l BuildLimits) (*Index, error) {
	if err := indexContext(ctx); err != nil {
		return nil, err
	}
	if !validBuildLimits(l) {
		return nil, ErrInvalid
	}
	if len(c.Lengths) > l.MaxPassages || len(c.Words) > l.MaxVocabulary || c.TotalTerms > l.MaxTerms {
		return nil, ErrLimit
	}
	if c.TotalTerms < 0 {
		return nil, ErrInvalid
	}
	bytesLeft, keysLeft := l.MaxInputBytes, l.MaxKeyBytes
	charge := func(s string) error {
		if len(s) > bytesLeft || len(s) > keysLeft {
			return ErrLimit
		}
		bytesLeft -= len(s)
		keysLeft -= len(s)
		return nil
	}
	for _, s := range []string{c.TermsVersion, c.Edition} {
		if err := charge(s); err != nil {
			return nil, err
		}
	}
	if !validString(c.TermsVersion) || !validString(c.Edition) {
		return nil, ErrInvalid
	}
	lengths := make([]int, len(c.Lengths))
	termsLeft, rowsLeft := l.MaxTerms, l.MaxPostings
	for wi, w := range c.Words {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if len(w.Term) > keysLeft || len(w.Rows) > rowsLeft {
			return nil, ErrLimit
		}
		if !validString(w.Term) || wi > 0 && c.Words[wi-1].Term >= w.Term || len(w.Rows) == 0 {
			return nil, ErrInvalid
		}
		keysLeft -= len(w.Term)
		rowsLeft -= len(w.Rows)
		previous := -1
		for _, r := range w.Rows {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if r.Passage <= previous || r.Passage < 0 || r.Passage >= len(lengths) {
				return nil, ErrInvalid
			}
			previous = r.Passage
			frequency := 0
			for _, n := range [3]int{r.Body, r.Headings, r.Labels} {
				if n < 0 {
					return nil, ErrInvalid
				}
				if n > termsLeft {
					return nil, ErrLimit
				}
				termsLeft -= n
				frequency += n
			}
			if frequency == 0 {
				return nil, ErrInvalid
			}
			if frequency > bytesLeft/len(w.Term) {
				return nil, ErrLimit
			}
			bytesLeft -= frequency * len(w.Term)
			lengths[r.Passage] += frequency // all additions together fit the admitted MaxTerms.
		}
	}
	total := l.MaxTerms - termsLeft
	if total != c.TotalTerms || !slices.Equal(lengths, c.Lengths) {
		return nil, ErrInvalid
	}
	e := &indexedEdition{terms: make(map[string]*postingList, len(c.Words)), lengths: lengths, totalLength: total}
	for _, w := range c.Words {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		rows := make([]posting, len(w.Rows))
		for i, r := range w.Rows {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			rows[i] = posting{r.Passage, r.Body, r.Headings, r.Labels}
		}
		e.terms[strings.Clone(w.Term)] = &postingList{rows: rows}
	}
	d := &indexData{version: strings.Clone(c.TermsVersion), editions: map[string]*indexedEdition{strings.Clone(c.Edition): e}, input: inputSize{editions: 1, passages: len(lengths), terms: total, bytes: l.MaxInputBytes - bytesLeft}}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &Index{data: d}, nil
}
