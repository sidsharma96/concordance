package htmlpassagemap

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"strings"
	"unicode/utf8"
)

// This byte grammar belongs to Format, not the current Go struct representation.
// Length/count framing preserves empty fields, duplicates and term/field order.
// SHA-256 Write cannot fail; no concatenated copy of all fields is allocated.
func fingerprint(ctx context.Context, f Fields, l Limits) ([sha256.Size]byte, error) {
	zero := [sha256.Size]byte{}
	if f.PassageProcessor != passageVersion || f.Processor != fieldVersion || f.TermsVersion != termsVersion {
		return zero, ErrUnsupported
	}
	if len(f.Passages) == 0 {
		return zero, ErrInvalid
	}
	if len(f.Passages) > l.MaxPassages {
		return zero, ErrLimit
	}
	h := sha256.New()
	_, _ = h.Write([]byte("concordance.html-fields/v1\x00"))
	var number [8]byte
	count := func(n int) { binary.BigEndian.PutUint64(number[:], uint64(n)); _, _ = h.Write(number[:]) }
	remaining, terms := l.MaxFieldBytes, l.MaxTerms
	text := func(s string, isTerm bool) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if len(s) > remaining || isTerm && len(s) > l.MaxTermBytes {
			return ErrLimit
		}
		if s == "" || !utf8.ValidString(s) || strings.IndexByte(s, 0) >= 0 {
			return ErrInvalid
		}
		remaining -= len(s)
		count(len(s))
		_, _ = h.Write([]byte(s))
		return nil
	}
	for _, s := range []string{f.PassageProcessor, f.Processor, f.TermsVersion} {
		if err := text(s, false); err != nil {
			return zero, err
		}
	}
	count(len(f.Passages))
	for _, p := range f.Passages {
		if err := ctx.Err(); err != nil {
			return zero, err
		}
		for _, field := range [3][]string{p.Body, p.Headings, p.Labels} {
			if len(field) > terms {
				return zero, ErrLimit
			}
			terms -= len(field)
			count(len(field))
			for _, s := range field {
				if err := text(s, true); err != nil {
					return zero, err
				}
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	var sum [sha256.Size]byte
	copy(sum[:], h.Sum(nil))
	return sum, nil
}
