package htmlsearch

import (
	"context"
	"strings"
	"unicode/utf8"
)

func validLimits(l Limits) bool {
	p, f := l.Fields.Passages, l.Fields.Field
	return l.MaxEditions > 0 && l.MaxArticles > 0 && l.MaxBlocks > 0 && l.MaxGroups > 0 && l.MaxInputBytes > 0 &&
		l.MaxPassages > 0 && l.MaxTerms > 0 && l.MaxOutputBytes > 0 &&
		l.Fields.MaxViewBytes > 0 && l.Fields.MaxTerms > 0 && l.Fields.MaxOutputBytes > 0 &&
		f.MaxInputBytes > 0 && f.MaxTerms > 0 && f.MaxTermBytes > 0 && f.MaxOutputBytes > 0 &&
		p.MaxBlocks > 0 && p.MaxGroups > 0 && p.MaxInputBytes > 0 && p.MaxPassages > 0 &&
		p.MaxBodyBytes > 0 && p.MaxReferences > 0 && p.MaxContextBytes > 0
}

// Admit counts and logical string bytes before building lookup or prepared data.
// Subtraction counters avoid overflow even for caller limits near MaxInt. Original
// HTMLBytes is provenance, not the bytes allocated by this retained Article value.
func admit(ctx context.Context, input []Edition, l Limits) error {
	if len(input) > l.MaxEditions {
		return ErrLimit
	}
	articles, blocks, groups, bytes := l.MaxArticles, l.MaxBlocks, l.MaxGroups, l.MaxInputBytes
	charge := func(s string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if len(s) > bytes {
			return ErrLimit
		}
		bytes -= len(s)
		return nil
	}
	for _, edition := range input {
		if err := charge(edition.ID); err != nil {
			return err
		}
		if len(edition.Articles) > articles {
			return ErrLimit
		}
		articles -= len(edition.Articles)
		for _, article := range edition.Articles {
			if len(article.Blocks) > blocks || len(article.Groups) > groups {
				return ErrLimit
			}
			blocks -= len(article.Blocks)
			groups -= len(article.Groups)
			for _, s := range []string{article.Processor, article.Profile} {
				if err := charge(s); err != nil {
					return err
				}
			}
			for _, block := range article.Blocks {
				for _, s := range []string{string(block.Kind), block.Text, block.Anchor, block.SourceURL} {
					if err := charge(s); err != nil {
						return err
					}
				}
			}
			for _, group := range article.Groups {
				for _, s := range []string{string(group.Kind), group.Label} {
					if err := charge(s); err != nil {
						return err
					}
				}
			}
		}
	}
	return ctx.Err()
}

func editionsValid(ctx context.Context, input []Edition) error {
	seen := make(map[string]struct{}, len(input))
	for _, edition := range input {
		if err := ctx.Err(); err != nil {
			return err
		}
		if edition.ID == "" || !utf8.ValidString(edition.ID) || strings.IndexByte(edition.ID, 0) >= 0 {
			return ErrInvalid
		}
		if _, exists := seen[edition.ID]; exists {
			return ErrInvalid
		}
		seen[edition.ID] = struct{}{}
	}
	return ctx.Err()
}
