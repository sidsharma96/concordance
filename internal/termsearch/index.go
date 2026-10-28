package termsearch

import (
	"context"
	"strings"
)

// BuildLimits bounds original input and owned index cardinalities. All values are
// positive logical ceilings, not exact heap/CPU quotas. InputBytes includes the
// corpus version, edition IDs and every term occurrence. Vocabulary counts distinct
// words per edition; Postings counts distinct word/passage pairs. KeyBytes counts
// cloned version/edition/word bytes. Maps, list capacity and pointers add overhead.
type BuildLimits struct {
	MaxEditions   int
	MaxPassages   int
	MaxTerms      int
	MaxInputBytes int
	MaxVocabulary int
	MaxPostings   int
	MaxKeyBytes   int
}

// Index owns immutable counts, lengths, statistics and cloned string keys. NewIndex
// borrows input unchanged until return, retaining no caller arrays or string backing
// allocations. Copies share immutable state. Read-only queries use local scratch and
// may run concurrently. No I/O, goroutines or Close. The zero value is invalid.
// Passage positions preserve the original input order, not durable identity or
// authorization. This is an in-memory representation, not a persistent index format.
type Index struct{ data *indexData }

type indexData struct {
	version  string
	editions map[string]*indexedEdition
	input    inputSize
}
type inputSize struct{ editions, passages, terms, bytes int }
type indexedEdition struct {
	terms       map[string]*postingList
	lengths     []int
	totalLength int
}
type postingList struct{ rows []posting }
type posting struct {
	passage                int
	body, headings, labels int
}

func (p posting) frequency() int { return p.body + p.headings + p.labels }

func indexContext(ctx context.Context) error {
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
func validBuildLimits(l BuildLimits) bool {
	return l.MaxEditions > 0 && l.MaxPassages > 0 && l.MaxTerms > 0 && l.MaxInputBytes > 0 && l.MaxVocabulary > 0 && l.MaxPostings > 0 && l.MaxKeyBytes > 0
}

// NewIndex admits and validates ALL editions, then visits passages in input order.
// Repeated words update the last row; new word/passage pairs append exactly once,
// so lists are sorted without sorting postings. Counts retain the three fields.
// Failures return nil, including cancellation and late storage-limit overflow.
//
// Expected work is linear in input bytes/occurrences plus passages and retained
// records under ordinary hash-map assumptions. Owned state is O(passages + vocabulary
// + postings + key bytes). Hash/string comparison and map worst cases matter; these
// are not constant-time worst-case or exact retained-memory claims.
func NewIndex(ctx context.Context, c Corpus, l BuildLimits) (*Index, error) {
	if err := indexContext(ctx); err != nil {
		return nil, err
	}
	if !validBuildLimits(l) {
		return nil, ErrInvalid
	}
	size, err := admitIndex(ctx, c, l)
	if err != nil {
		return nil, err
	}
	remainingKeys := l.MaxKeyBytes
	ownKey := func(s string) (string, error) {
		if len(s) > remainingKeys {
			return "", ErrLimit
		}
		remainingKeys -= len(s)
		return strings.Clone(s), nil
	}
	version, err := ownKey(c.TermsVersion)
	if err != nil {
		return nil, err
	}
	d := &indexData{version: version, editions: make(map[string]*indexedEdition, len(c.Editions)), input: size}
	vocabulary, postings := l.MaxVocabulary, l.MaxPostings
	for _, edition := range c.Editions {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		id, err := ownKey(edition.ID)
		if err != nil {
			return nil, err
		}
		e := &indexedEdition{terms: make(map[string]*postingList), lengths: make([]int, len(edition.Passages))}
		d.editions[id] = e
		for pi, p := range edition.Passages {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			for field, terms := range [3][]string{p.Body, p.Headings, p.Labels} {
				for _, term := range terms {
					if err := ctx.Err(); err != nil {
						return nil, err
					}
					list := e.terms[term]
					if list == nil {
						if vocabulary == 0 {
							return nil, ErrLimit
						}
						vocabulary--
						key, err := ownKey(term)
						if err != nil {
							return nil, err
						}
						list = &postingList{}
						// Assign only the cloned key. Later updates mutate the list, not the map
						// key, so equal caller strings cannot replace this owned key's storage.
						e.terms[key] = list
					}
					if len(list.rows) == 0 || list.rows[len(list.rows)-1].passage != pi {
						if postings == 0 {
							return nil, ErrLimit
						}
						postings--
						list.rows = append(list.rows, posting{passage: pi})
					}
					row := &list.rows[len(list.rows)-1]
					switch field {
					case 0:
						row.body++
					case 1:
						row.headings++
					case 2:
						row.labels++
					}
					// All occurrences were admitted under MaxTerms before these additions.
					e.lengths[pi]++
					e.totalLength++
				}
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &Index{data: d}, nil
}

func admitIndex(ctx context.Context, c Corpus, l BuildLimits) (inputSize, error) {
	zero := inputSize{}
	if len(c.Editions) > l.MaxEditions {
		return zero, ErrLimit
	}
	size := inputSize{editions: len(c.Editions)}
	remaining := l.MaxInputBytes
	charge := func(s string) error {
		if len(s) > remaining {
			return ErrLimit
		}
		remaining -= len(s)
		if !validString(s) {
			return ErrInvalid
		}
		return nil
	}
	if err := charge(c.TermsVersion); err != nil {
		return zero, err
	}
	seen := make(map[string]struct{})
	for _, e := range c.Editions {
		if err := ctx.Err(); err != nil {
			return zero, err
		}
		if err := charge(e.ID); err != nil {
			return zero, err
		}
		if _, ok := seen[e.ID]; ok {
			return zero, ErrInvalid
		}
		seen[e.ID] = struct{}{}
		if len(e.Passages) > l.MaxPassages-size.passages {
			return zero, ErrLimit
		}
		size.passages += len(e.Passages)
		for _, p := range e.Passages {
			if err := ctx.Err(); err != nil {
				return zero, err
			}
			for _, field := range [3][]string{p.Body, p.Headings, p.Labels} {
				if len(field) > l.MaxTerms-size.terms {
					return zero, ErrLimit
				}
				size.terms += len(field)
				for _, term := range field {
					if err := ctx.Err(); err != nil {
						return zero, err
					}
					if err := charge(term); err != nil {
						return zero, err
					}
				}
			}
		}
	}
	size.bytes = l.MaxInputBytes - remaining
	return size, ctx.Err()
}
