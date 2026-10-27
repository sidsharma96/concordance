// Package termsearch finds passage candidates in caller-prepared terms.
// It does not tokenize raw text, rank answers, persist an index or publish data.
package termsearch

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"
)

var (
	ErrInvalid      = errors.New("invalid term search input")
	ErrLimit        = errors.New("term search limit exceeded")
	ErrIncompatible = errors.New("incompatible term versions")
)

// Fields keeps terms from a passage body and its supplied context separate.
// MatchAll considers their union. Strings are exact term keys: this package does
// not split, trim, fold case, normalize Unicode, stem or expand them.
type Fields struct {
	Body     []string
	Headings []string
	Labels   []string
}

// Edition is one exact caller-owned selection key and its ordered passages.
// An empty edition is valid. ID is not interpreted as SemVer or a mutable alias.
type Edition struct {
	ID       string
	Passages []Fields
}

// Corpus is a temporary matching input, not a persisted source schema. Edition
// IDs must be unique within it. TermsVersion identifies the caller's term rules;
// equal tags prevent accidental mixing, but cannot prove correct preparation.
type Corpus struct {
	TermsVersion string
	Editions     []Edition
}

// Query supplies already prepared keywords and one exact edition key. At least
// one nonempty term is required. Repeated terms do not add matching requirements.
type Query struct {
	TermsVersion string
	Edition      string
	Terms        []string
}

// Limits are positive caller-chosen admission and result ceilings, not exact
// heap or CPU limits. Counts cover all editions, including unselected ones.
// MaxTerms counts every corpus term occurrence across all fields. MaxQueryTerms
// counts provided query terms before deduplication. MaxInputBytes counts every
// string occurrence in Corpus and Query, including version tags and edition IDs;
// repeated string references are charged again. MaxMatches never truncates results.
type Limits struct {
	MaxEditions   int
	MaxPassages   int
	MaxTerms      int
	MaxQueryTerms int
	MaxInputBytes int
	MaxMatches    int
}

// Result distinguishes a missing edition from a known edition with no matches.
// Passages owns its storage and contains positions within the selected edition's
// passage order in this exact input. These positions are not persistent IDs or
// citations; the caller keeps their association with its source metadata.
type Result struct {
	EditionFound bool
	Passages     []int
}

// MatchAll returns every passage containing every distinct query term across its
// supplied fields, in input order. Terms in different passages never combine.
// A missing edition returns no matches and never selects another edition.
//
// The caller provides authorized input and keeps its slices/strings unchanged
// until return. No input is retained afterward, and no goroutines or I/O start.
// A deadline is required; cancellation is cooperative. All input is admitted and
// validated before matching, even unselected editions. Every failure, including
// excessive matches or late cancellation, returns a zero Result.
func MatchAll(ctx context.Context, corpus Corpus, query Query, limits Limits) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if _, ok := ctx.Deadline(); !ok || !validLimits(limits) {
		return Result{}, ErrInvalid
	}
	if err := admit(ctx, corpus, query, limits); err != nil {
		return Result{}, err
	}
	selected, err := validate(ctx, corpus, query)
	if err != nil {
		return Result{}, err
	}
	if selected < 0 {
		return Result{}, ctx.Err()
	}

	positions := make(map[string]int, len(query.Terms))
	for _, term := range query.Terms {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		if _, exists := positions[term]; !exists {
			positions[term] = len(positions)
		}
	}
	found := make([]bool, len(positions))
	result := Result{EditionFound: true}
	for index, passage := range corpus.Editions[selected].Passages {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		clear(found)
		match, err := matchesAll(ctx, passage, positions, found)
		if err != nil {
			return Result{}, err
		}
		if match {
			if len(result.Passages) >= limits.MaxMatches {
				return Result{}, ErrLimit
			}
			result.Passages = append(result.Passages, index)
		}
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	return result, nil
}

func validLimits(l Limits) bool {
	return l.MaxEditions > 0 && l.MaxPassages > 0 && l.MaxTerms > 0 && l.MaxQueryTerms > 0 && l.MaxInputBytes > 0 && l.MaxMatches > 0
}

// Count before allocating lookup/output buffers. Subtraction avoids counter
// overflow even when caller limits approach the largest representable integer.
func admit(ctx context.Context, c Corpus, q Query, l Limits) error {
	if len(c.Editions) > l.MaxEditions || len(q.Terms) > l.MaxQueryTerms {
		return ErrLimit
	}
	if len(q.Terms) == 0 {
		return ErrInvalid
	}
	remaining := l.MaxInputBytes
	charge := func(n int) error {
		if n > remaining {
			return ErrLimit
		}
		remaining -= n
		return nil
	}
	for _, s := range []string{c.TermsVersion, q.TermsVersion, q.Edition} {
		if err := charge(len(s)); err != nil {
			return err
		}
	}
	for _, term := range q.Terms {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := charge(len(term)); err != nil {
			return err
		}
	}
	passages, terms := 0, 0
	for _, edition := range c.Editions {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := charge(len(edition.ID)); err != nil {
			return err
		}
		if len(edition.Passages) > l.MaxPassages-passages {
			return ErrLimit
		}
		passages += len(edition.Passages)
		for _, passage := range edition.Passages {
			if err := ctx.Err(); err != nil {
				return err
			}
			for _, field := range [3][]string{passage.Body, passage.Headings, passage.Labels} {
				if len(field) > l.MaxTerms-terms {
					return ErrLimit
				}
				terms += len(field)
				for _, term := range field {
					if err := ctx.Err(); err != nil {
						return err
					}
					if err := charge(len(term)); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

func validate(ctx context.Context, c Corpus, q Query) (int, error) {
	if !validString(c.TermsVersion) || !validString(q.TermsVersion) || !validString(q.Edition) {
		return -1, ErrInvalid
	}
	if c.TermsVersion != q.TermsVersion {
		return -1, ErrIncompatible
	}
	for _, term := range q.Terms {
		if err := ctx.Err(); err != nil {
			return -1, err
		}
		if !validString(term) {
			return -1, ErrInvalid
		}
	}
	seen := make(map[string]struct{}, len(c.Editions))
	selected := -1
	for i, edition := range c.Editions {
		if err := ctx.Err(); err != nil {
			return -1, err
		}
		if !validString(edition.ID) {
			return -1, ErrInvalid
		}
		if _, exists := seen[edition.ID]; exists {
			return -1, ErrInvalid
		}
		seen[edition.ID] = struct{}{}
		if edition.ID == q.Edition {
			selected = i
		}
		for _, passage := range edition.Passages {
			if err := ctx.Err(); err != nil {
				return -1, err
			}
			for _, field := range [3][]string{passage.Body, passage.Headings, passage.Labels} {
				for _, term := range field {
					if err := ctx.Err(); err != nil {
						return -1, err
					}
					if !validString(term) {
						return -1, ErrInvalid
					}
				}
			}
		}
	}
	return selected, ctx.Err()
}
func validString(s string) bool { return s != "" && utf8.ValidString(s) && strings.IndexByte(s, 0) < 0 }

func matchesAll(ctx context.Context, p Fields, positions map[string]int, found []bool) (bool, error) {
	count := 0
	for _, field := range [3][]string{p.Body, p.Headings, p.Labels} {
		for _, term := range field {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			if position, exists := positions[term]; exists && !found[position] {
				found[position] = true
				count++
				if count == len(positions) {
					return true, nil
				}
			}
		}
	}
	return false, nil
}
