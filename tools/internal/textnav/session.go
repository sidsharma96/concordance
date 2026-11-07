package textnav

import (
	"context"
	"strings"
	"unicode/utf8"
)

// Limits apply to one sequential session. Attempted invalid/canceled operations
// consume a call. Returned bytes count only actual Read text, including repeats;
// transport metadata, search snippets and model tokens need their own admission.
type Limits struct {
	MaxCalls         int
	MaxReturnedBytes int
	MaxLiteralBytes  int
	MaxFindMatches   int
}

type Stats struct {
	Calls         int
	ReturnedBytes int
}

// Session owns mutable counters and is not safe for concurrent calls. Different
// sessions may share one immutable Corpus. No context is retained between calls.
type Session struct {
	corpus *Corpus
	limits Limits
	stats  Stats
}

func (c *Corpus) NewSession(l Limits) (*Session, error) {
	if c == nil || c.documents == nil || l.MaxCalls <= 0 || l.MaxReturnedBytes <= 0 || l.MaxLiteralBytes <= 0 || l.MaxFindMatches <= 0 {
		return nil, ErrInvalid
	}
	return &Session{corpus: c, limits: l}, nil
}

func (s *Session) Stats() Stats {
	if s == nil {
		return Stats{}
	}
	return s.stats
}

func (s *Session) begin(ctx context.Context) error {
	if s == nil || s.corpus == nil {
		return ErrInvalid
	}
	if s.stats.Calls >= s.limits.MaxCalls {
		return ErrLimit
	}
	s.stats.Calls++
	return checkContext(ctx)
}

// Lookup is exact and returns declared identity/length from the verified corpus.
// It returns no document body and charges no returned source bytes.
func (s *Session) Lookup(ctx context.Context, id string) (Document, error) {
	if err := s.begin(ctx); err != nil {
		return Document{}, err
	}
	d, ok := s.corpus.documents[id]
	if !ok {
		return Document{}, ErrMissing
	}
	return Document{Ref: d.ref, Bytes: len(d.text)}, nil
}

func (s *Session) resolve(ref Ref) (document, error) {
	d, ok := s.corpus.documents[ref.ID]
	if !ok {
		return document{}, ErrMissing
	}
	if d.ref.SHA256 != ref.SHA256 {
		return document{}, ErrBinding
	}
	return d, nil
}

// Span is an exact half-open interval of raw UTF-8 bytes, not rune or line indices.
type Span struct {
	Start int
	End   int
}

// Excerpt.Text is an immutable substring and may retain this document's complete
// backing storage. The Ref and Span identify its exact source, without normalization.
type Excerpt struct {
	Ref  Ref
	Span Span
	Text string
}

func (s *Session) Read(ctx context.Context, ref Ref, span Span) (Excerpt, error) {
	if err := s.begin(ctx); err != nil {
		return Excerpt{}, err
	}
	d, err := s.resolve(ref)
	if err != nil {
		return Excerpt{}, err
	}
	if span.Start < 0 || span.Start >= span.End || span.End > len(d.text) || !boundary(d.text, span.Start) || !boundary(d.text, span.End) {
		return Excerpt{}, ErrInvalid
	}
	n := span.End - span.Start
	if n > s.limits.MaxReturnedBytes-s.stats.ReturnedBytes {
		return Excerpt{}, ErrLimit
	}
	if err := ctx.Err(); err != nil {
		return Excerpt{}, err
	}
	s.stats.ReturnedBytes += n
	return Excerpt{Ref: d.ref, Span: span, Text: d.text[span.Start:span.End]}, nil
}

func boundary(text string, at int) bool {
	return at >= 0 && at <= len(text) && (at == len(text) || utf8.RuneStart(text[at]))
}

type FindResult struct {
	Matches []Span
	Next    int
	More    bool
}

// Find returns a bounded page of literal, case-sensitive, non-overlapping matches
// at or after from. It does not return body/context text. Next is the end of the
// last returned match when More, otherwise the document length. The caller owns
// Matches. A later page starts at Next; there is no regex or inferred section map.
func (s *Session) Find(ctx context.Context, ref Ref, literal string, from, limit int) (FindResult, error) {
	if err := s.begin(ctx); err != nil {
		return FindResult{}, err
	}
	d, err := s.resolve(ref)
	if err != nil {
		return FindResult{}, err
	}
	if literal == "" || !utf8.ValidString(literal) || strings.ContainsRune(literal, 0) || !boundary(d.text, from) || limit <= 0 {
		return FindResult{}, ErrInvalid
	}
	if len(literal) > s.limits.MaxLiteralBytes || limit > s.limits.MaxFindMatches {
		return FindResult{}, ErrLimit
	}
	r := FindResult{Matches: make([]Span, 0, limit), Next: len(d.text)}
	at := from
	for len(r.Matches) < limit {
		if err := ctx.Err(); err != nil {
			return FindResult{}, err
		}
		i := strings.Index(d.text[at:], literal)
		if i < 0 {
			if err := ctx.Err(); err != nil {
				return FindResult{}, err
			}
			return r, nil
		}
		start := at + i
		at = start + len(literal)
		r.Matches = append(r.Matches, Span{Start: start, End: at})
	}
	// One lookahead distinguishes a page from an exhaustive result. It returns
	// only an existence bit, never an uncharged extra snippet or location.
	if err := ctx.Err(); err != nil {
		return FindResult{}, err
	}
	r.More = strings.Index(d.text[at:], literal) >= 0
	if r.More {
		r.Next = at
	}
	if err := ctx.Err(); err != nil {
		return FindResult{}, err
	}
	return r, nil
}
