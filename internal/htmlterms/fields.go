// Package htmlterms prepares separate matching fields from retained HTML articles.
// It does not fetch, rank, choose editions, persist citations or publish data.
package htmlterms

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/sidsharma96/concordance/internal/htmlarticle"
	"github.com/sidsharma96/concordance/internal/htmlpassage"
	"github.com/sidsharma96/concordance/internal/termsearch"
	"github.com/sidsharma96/concordance/internal/textterms"
)

const Processor = "htmlterms/v1"

var (
	ErrInvalid = errors.New("invalid HTML field preparation input")
	ErrLimit   = errors.New("HTML field preparation limit exceeded")
)

// Limits composes passage admission and per-field text preparation limits.
// MaxViewBytes counts projected raw text plus LF separators across every field,
// including repeated heading/label context. MaxTerms and MaxOutputBytes bound
// aggregate prepared occurrences/term bytes after each bounded field preparation.
// Temporary output can include the prior admitted aggregate plus one Field result;
// these are logical ceilings, not exact heap quotas. All limits must be positive.
type Limits struct {
	Passages       htmlpassage.Limits
	Field          textterms.Limits
	MaxViewBytes   int
	MaxTerms       int
	MaxOutputBytes int
}

// Prepared owns its plan metadata, field slices and term strings, not an article.
// Fields[i] corresponds to Plan.Passages[i] in the exact unchanged input article.
// The caller retains that association and source/edition/revision/profile metadata;
// positions are not persistent identities, authorization or a complete citation.
// Processor versions projection separately from Plan.Processor and TermsVersion.
type Prepared struct {
	Processor    string
	TermsVersion string
	Plan         htmlpassage.Plan
	Fields       []termsearch.Fields
}

// Build constructs the canonical passage plan, then prepares body block text,
// every referenced ancestor heading, and nonempty referenced group labels as
// separate fields. Successive strings in each field are joined by one LF. Leading
// headings already in a body are not added again to ancestor context. Image alt
// text is included as block Text; image URLs and inferred descriptions are not.
//
// The caller keeps article unchanged during the call and for interpreting returned
// plan indices. No input buffers are retained. A deadline is required; cancellation
// is cooperative and every failure returns a zero Prepared. Wrapped passage/text
// errors preserve errors.Is. No I/O, goroutines or cache are created.
func Build(ctx context.Context, article htmlarticle.Article, limits Limits) (Prepared, error) {
	if err := ctx.Err(); err != nil {
		return Prepared{}, err
	}
	if _, ok := ctx.Deadline(); !ok || !validLimits(limits) {
		return Prepared{}, ErrInvalid
	}
	plan, err := htmlpassage.Build(ctx, article, limits.Passages)
	if err != nil {
		return Prepared{}, fmt.Errorf("build HTML passage plan: %w", err)
	}
	// Plan allocations have their own passage limits. Admit all repeated raw
	// views before allocating joined text or prepared field output.
	if err := admitViews(ctx, article, plan, limits); err != nil {
		return Prepared{}, err
	}
	fields := make([]termsearch.Fields, len(plan.Passages))
	remainingTerms, remainingBytes := limits.MaxTerms, limits.MaxOutputBytes
	for index, passage := range plan.Passages {
		targets := [3]*[]string{&fields[index].Body, &fields[index].Headings, &fields[index].Labels}
		for kind, target := range targets {
			var view strings.Builder
			first := true
			err := walkField(ctx, article, passage, kind, func(text string) error {
				if !first {
					view.WriteByte('\n')
				}
				first = false
				view.WriteString(text)
				return nil
			})
			if err != nil {
				return Prepared{}, err
			}
			prepared, err := textterms.Prepare(ctx, view.String(), limits.Field)
			if err != nil {
				return Prepared{}, fmt.Errorf("prepare HTML passage %d field %d: %w", index, kind, err)
			}
			if len(prepared.Terms) > remainingTerms {
				return Prepared{}, ErrLimit
			}
			remainingTerms -= len(prepared.Terms)
			for _, term := range prepared.Terms {
				if err := ctx.Err(); err != nil {
					return Prepared{}, err
				}
				if len(term) > remainingBytes {
					return Prepared{}, ErrLimit
				}
				remainingBytes -= len(term)
			}
			*target = prepared.Terms
		}
	}
	if err := ctx.Err(); err != nil {
		return Prepared{}, err
	}
	return Prepared{Processor: Processor, TermsVersion: textterms.Version, Plan: plan, Fields: fields}, nil
}

func validLimits(l Limits) bool {
	return l.MaxViewBytes > 0 && l.MaxTerms > 0 && l.MaxOutputBytes > 0 &&
		l.Field.MaxInputBytes > 0 && l.Field.MaxTerms > 0 && l.Field.MaxTermBytes > 0 && l.Field.MaxOutputBytes > 0
}

func admitViews(ctx context.Context, a htmlarticle.Article, plan htmlpassage.Plan, limits Limits) error {
	remaining := limits.MaxViewBytes
	for _, p := range plan.Passages {
		for kind := range 3 {
			fieldBytes, first := limits.Field.MaxInputBytes, true
			err := walkField(ctx, a, p, kind, func(text string) error {
				if !first {
					if remaining == 0 || fieldBytes == 0 {
						return ErrLimit
					}
					remaining--
					fieldBytes--
				}
				first = false
				if len(text) > remaining || len(text) > fieldBytes {
					return ErrLimit
				}
				remaining -= len(text)
				fieldBytes -= len(text)
				return nil
			})
			if err != nil {
				return err
			}
		}
	}
	return ctx.Err()
}

// Inputs are only the validated plan built inside Build, never caller-supplied
// ranges. Keep raw field traversal shared by measurement and materialization.
func walkField(ctx context.Context, a htmlarticle.Article, p htmlpassage.Passage, kind int, emit func(string) error) error {
	visit := func(text string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		return emit(text)
	}
	switch kind {
	case 0:
		for _, b := range a.Blocks[p.Body.First:p.Body.After] {
			if err := visit(b.Text); err != nil {
				return err
			}
		}
	case 1:
		for _, i := range p.Headings {
			if err := visit(a.Blocks[i].Text); err != nil {
				return err
			}
		}
	case 2:
		for _, i := range p.Groups {
			if err := ctx.Err(); err != nil {
				return err
			}
			if text := a.Groups[i].Label; text != "" {
				if err := emit(text); err != nil {
					return err
				}
			}
		}
	}
	return ctx.Err()
}
