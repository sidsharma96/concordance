package preparedimport

import (
	"context"
	"path"
	"strings"
	"unicode"
	"unicode/utf8"
)

func identifier(s string) bool {
	return s != "" && len(s) <= 256 && utf8.ValidString(s) && strings.IndexFunc(s, unicode.IsControl) < 0
}
func cleanPath(s string) bool {
	return s != "" && s != "." && s != ".." && !strings.HasPrefix(s, "/") && !strings.HasPrefix(s, "../") && !strings.Contains(s, "\\") && strings.IndexFunc(s, unicode.IsControl) < 0 && path.Clean(s) == s
}
func ready(ctx context.Context) error {
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
func admitText(s string, max int, left *int) error {
	if len(s) > max || len(s) > *left {
		return ErrLimit
	}
	if !utf8.ValidString(s) || strings.IndexByte(s, 0) >= 0 {
		return ErrInvalid
	}
	*left -= len(s)
	return nil
}
func take(n int, left *int) error {
	if n > *left {
		return ErrLimit
	}
	*left -= n
	return nil
}
func takeBytes(n int64, left *int64) error {
	if n > *left {
		return ErrLimit
	}
	*left -= n
	return nil
}

// Admit supplied counts and bytes before allocating membership/order maps or
// reading Git. Actual source membership is checked independently after capture.
func (i *Importer) admit(ctx context.Context, id string, editions []Edition) (int64, error) {
	l := i.limits
	if !identifier(id) {
		return 0, ErrInvalid
	}
	if len(editions) > l.Manifest.MaxEditions {
		return 0, ErrLimit
	}
	captures, docs, files := l.Manifest.MaxCaptures, l.Manifest.MaxDocuments, l.Manifest.MaxFiles
	values, input := l.Manifest.MaxValueBytes, l.MaxInputBytes
	text := func(s string) error { return admitText(s, l.Manifest.MaxStringBytes, &values) }
	if err := text(id); err != nil {
		return 0, err
	}
	for _, e := range editions {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		if !identifier(e.ID) {
			return 0, ErrInvalid
		}
		if err := text(e.ID); err != nil {
			return 0, err
		}
		if err := take(len(e.Captures), &captures); err != nil {
			return 0, err
		}
		for _, c := range e.Captures {
			if err := ctx.Err(); err != nil {
				return 0, err
			}
			if err := take(len(c.Pages), &docs); err != nil {
				return 0, err
			}
			if err := take(len(c.Pages), &files); err != nil {
				return 0, err
			}
			if err := take(len(c.Dependencies), &files); err != nil {
				return 0, err
			}
			for _, s := range []string{c.Source, c.Revision, c.Directory, c.Preparation, c.Toolchain, c.Profile.ID, c.Profile.ArticleClass} {
				if err := text(s); err != nil {
					return 0, err
				}
			}
			if c.Preparation == "" || c.Toolchain == "" || len(c.Recipe) == 0 || c.Directory != "." && !cleanPath(c.Directory) {
				return 0, ErrInvalid
			}
			for _, ss := range [][]string{c.Profile.OmitClasses, c.Profile.OmitIDs, c.Profile.NoticeClasses} {
				if len(ss) > 32 {
					return 0, ErrLimit
				}
				for _, s := range ss {
					if err := text(s); err != nil {
						return 0, err
					}
				}
			}
			if err := takeBytes(int64(len(c.Recipe)), &input); err != nil {
				return 0, err
			}
			for _, p := range c.Pages {
				if err := ctx.Err(); err != nil {
					return 0, err
				}
				if err := text(p.Path); err != nil {
					return 0, err
				}
				if !cleanPath(p.Path) || !strings.HasSuffix(p.Path, ".md") || len(p.HTML) == 0 {
					return 0, ErrInvalid
				}
				if err := takeBytes(int64(len(p.HTML)), &input); err != nil {
					return 0, err
				}
			}
			for _, p := range c.Dependencies {
				if err := text(p); err != nil {
					return 0, err
				}
				if !cleanPath(p) {
					return 0, ErrInvalid
				}
			}
		}
	}
	// Counts and strings are admitted; maps below are bounded by those counts.
	seen := make(map[string]bool, len(editions))
	sourceFiles := map[string]int{}
	sourceDocs := map[string]int{}
	for _, e := range editions {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		if seen[e.ID] {
			return 0, ErrInvalid
		}
		seen[e.ID] = true
		captured := make(map[string]bool, len(e.Captures))
		for _, c := range e.Captures {
			if _, ok := i.sources[c.Source]; !ok || captured[c.Source] {
				return 0, ErrInvalid
			}
			captured[c.Source] = true
			if len(c.Pages) > l.Manifest.MaxSourceDocuments-sourceDocs[c.Source] || len(c.Pages) > l.Manifest.MaxSourceFiles-sourceFiles[c.Source] {
				return 0, ErrLimit
			}
			sourceDocs[c.Source] += len(c.Pages)
			sourceFiles[c.Source] += len(c.Pages)
			if len(c.Dependencies) > l.Manifest.MaxSourceFiles-sourceFiles[c.Source] {
				return 0, ErrLimit
			}
			sourceFiles[c.Source] += len(c.Dependencies)
			paths := make(map[string]bool, len(c.Pages)+len(c.Dependencies))
			for _, p := range c.Pages {
				if paths[p.Path] {
					return 0, ErrMembership
				}
				paths[p.Path] = true
			}
			for _, p := range c.Dependencies {
				if paths[p] {
					return 0, ErrInvalid
				}
				paths[p] = true
			}
		}
	}
	return input, ctx.Err()
}
