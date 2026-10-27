// Package textterms prepares bounded lexical terms for technical documentation.
// It does not parse code, choose passage fields, rank results or publish data.
package textterms

import (
	"context"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Version identifies the grammar, lowercase mapping and Unicode classification
// tables. A table upgrade changes compatibility even if the grammar stays fixed.
const Version = "technical/v1/unicode-" + unicode.Version

var (
	ErrInvalid = errors.New("invalid text preparation input")
	ErrLimit   = errors.New("text preparation limit exceeded")
)

// Limits are positive caller-chosen logical ceilings, not exact heap/CPU quotas.
// MaxTerms counts occurrences, including duplicates. MaxTermBytes bounds both
// each raw span and its lowercase output. MaxOutputBytes counts the sum of output
// term bytes, excluding slice/string headers and the constant version label.
type Limits struct {
	MaxInputBytes  int
	MaxTerms       int
	MaxTermBytes   int
	MaxOutputBytes int
}

// Prepared owns its slice and term strings; none retain the original input.
// Terms keeps occurrences in input order. Version must accompany the terms when
// used with a matching corpus/query; the tag is not proof of correct preparation.
type Prepared struct {
	Version string
	Terms   []string
}

// Prepare lowercases terms with unicode.ToLower, preserving whole technical names.
// A term starts with a Unicode letter, decimal digit or ASCII underscore. Those
// characters, combining marks, '+' and '#' continue it. ASCII '.', '-' and '/'
// continue only immediately before a character that can start a term. Other runes
// delimit terms. Thus "C++ readiness-probe v1.35." produces three whole terms;
// "a..b" produces two. This grammar is not a code, URL or command-line parser.
//
// There is no full case folding, Unicode normalization, stemming, stop-word
// removal, deduplication or component-word expansion. Empty/punctuation-only text
// is valid. Malformed UTF-8 and NUL are rejected, even outside retained terms.
//
// Input is borrowed unchanged until return. All admission/validation precedes
// output allocation. A deadline is required; cancellation is cooperative. Every
// failure, including late cancellation, returns a zero Prepared. No I/O, goroutines
// or retained state are created. Returned terms are not citations or source IDs.
func Prepare(ctx context.Context, text string, limits Limits) (Prepared, error) {
	if err := ctx.Err(); err != nil {
		return Prepared{}, err
	}
	if _, ok := ctx.Deadline(); !ok || !validLimits(limits) {
		return Prepared{}, ErrInvalid
	}
	if len(text) > limits.MaxInputBytes {
		return Prepared{}, ErrLimit
	}
	count, remaining := 0, limits.MaxOutputBytes
	err := scan(ctx, text, func(start, end int) error {
		if count == limits.MaxTerms || end-start > limits.MaxTermBytes {
			return ErrLimit
		}
		termBytes := limits.MaxTermBytes
		for _, r := range text[start:end] {
			if err := ctx.Err(); err != nil {
				return err
			}
			width := utf8.RuneLen(unicode.ToLower(r))
			if width > termBytes || width > remaining {
				return ErrLimit
			}
			termBytes -= width
			remaining -= width
		}
		count++
		return nil
	})
	if err != nil {
		return Prepared{}, err
	}
	var terms []string
	if count > 0 {
		terms = make([]string, 0, count)
	}
	err = scan(ctx, text, func(start, end int) error {
		var term strings.Builder
		for _, r := range text[start:end] {
			if err := ctx.Err(); err != nil {
				return err
			}
			term.WriteRune(unicode.ToLower(r))
		}
		terms = append(terms, term.String())
		return nil
	})
	if err != nil {
		return Prepared{}, err
	}
	if err := ctx.Err(); err != nil {
		return Prepared{}, err
	}
	return Prepared{Version: Version, Terms: terms}, nil
}

func validLimits(l Limits) bool {
	return l.MaxInputBytes > 0 && l.MaxTerms > 0 && l.MaxTermBytes > 0 && l.MaxOutputBytes > 0
}

func startsTerm(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

// scan validates every visited rune and emits maximal raw byte ranges without
// allocating token buffers. A failed emit or cancellation stops with an error.
func scan(ctx context.Context, text string, emit func(start, end int) error) error {
	start := -1
	for i := 0; i < len(text); {
		if err := ctx.Err(); err != nil {
			return err
		}
		r, width := utf8.DecodeRuneInString(text[i:])
		if r == 0 || (r == utf8.RuneError && width == 1) {
			return ErrInvalid
		}
		keep := startsTerm(r)
		if start >= 0 {
			keep = keep || unicode.IsMark(r) || r == '+' || r == '#'
			if r == '.' || r == '-' || r == '/' {
				next, _ := utf8.DecodeRuneInString(text[i+width:])
				keep = startsTerm(next)
			}
		}
		if keep {
			if start < 0 {
				start = i
			}
		} else if start >= 0 {
			if err := emit(start, i); err != nil {
				return err
			}
			start = -1
		}
		i += width
	}
	if start >= 0 {
		return emit(start, len(text))
	}
	return ctx.Err()
}
