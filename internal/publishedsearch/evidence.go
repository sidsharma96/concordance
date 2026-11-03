package publishedsearch

import (
	"context"
	"fmt"

	"github.com/sidsharma96/concordance/internal/htmlarticle"
	"github.com/sidsharma96/concordance/internal/htmlpassage"
	"github.com/sidsharma96/concordance/internal/objectstore"
	"github.com/sidsharma96/concordance/internal/termsearch"
)

// Evidence owns its quote string and context slices. Headings and notice labels
// are separate from the verbatim body; strings are immutable owned values, not
// aliases into the downloaded bytes. Copying this wrapper shares its slices.
type Evidence struct {
	Quote            string
	Headings, Labels []string
}

// EvidenceLimits bounds the complete canonical article plan and selected output.
// All values must be positive. MaxContextBytes counts selected heading and label
// bytes, excluding serialization separators. It is not a model-token limit.
type EvidenceLimits struct {
	Passages        htmlpassage.Limits
	MaxQuoteBytes   int
	MaxContextBytes int
}

func validEvidenceLimits(l EvidenceLimits) bool {
	p := l.Passages
	return l.MaxQuoteBytes > 0 && l.MaxContextBytes > 0 && p.MaxBlocks > 0 && p.MaxGroups > 0 && p.MaxInputBytes > 0 && p.MaxPassages > 0 && p.MaxBodyBytes > 0 && p.MaxReferences > 0 && p.MaxContextBytes > 0
}

type articleRead struct {
	body    []byte
	article htmlarticle.Article
}

// Share the strict read/provenance rules with Quote. The edition independently
// checks its exact article and saved range when it assembles the body afterward.
func (r *Reader) readArticle(ctx context.Context, ref Reference) (articleRead, error) {
	loc, err := r.Location(ctx, ref)
	if err != nil {
		return articleRead{}, err
	}
	if loc.Document.Article.Bytes > int64(r.data.article.MaxEncodedBytes) {
		return articleRead{}, ErrLimit
	}
	body, err := r.data.objects.Read(ctx, objectstore.Ref{SHA256: loc.Document.Article.SHA256, Bytes: loc.Document.Article.Bytes})
	if err != nil {
		return articleRead{}, fmt.Errorf("read quoted article: %w", err)
	}
	position, err := r.data.edition.Location(ref.passage)
	if err != nil {
		return articleRead{}, err
	}
	a, err := r.data.manifest.VerifyArticle(ctx, loc.Selection.Edition, position.Article, body, r.data.article)
	if err != nil {
		return articleRead{}, fmt.Errorf("verify quoted article: %w", err)
	}
	return articleRead{body: body, article: a}, nil
}

// Evidence reads one exact article, verifies manifest/index/body associations,
// then rebuilds its bounded canonical plan to recover the saved range's context.
// Exactly one canonical range must match. No model text, cache or goroutine is
// introduced. A failure returns zero Evidence, including after a successful read.
// Work includes the existing two article decodes plus one complete passage plan;
// plan scratch is bounded separately from the selected output. Reader/client
// lifetime and simultaneous-call admission remain the caller's responsibility.
func (r *Reader) Evidence(ctx context.Context, ref Reference, l EvidenceLimits) (Evidence, error) {
	if !validEvidenceLimits(l) {
		return Evidence{}, ErrInvalid
	}
	read, err := r.readArticle(ctx, ref)
	if err != nil {
		return Evidence{}, err
	}
	quote, err := r.data.edition.Quote(ctx, ref.passage, read.body, r.data.article, l.MaxQuoteBytes)
	if err != nil {
		return Evidence{}, err
	}
	plan, err := htmlpassage.Build(ctx, read.article, l.Passages)
	if err != nil {
		return Evidence{}, fmt.Errorf("build quote context: %w", err)
	}
	location, err := r.data.edition.Location(ref.passage)
	if err != nil {
		return Evidence{}, err
	}
	out := Evidence{Quote: quote}
	remaining := l.MaxContextBytes
	found := false
	for _, passage := range plan.Passages {
		if err := ctx.Err(); err != nil {
			return Evidence{}, err
		}
		if passage.Body != location.Body {
			continue
		}
		if found {
			return Evidence{}, ErrMismatch
		}
		found = true
		for _, index := range passage.Headings {
			if err := ctx.Err(); err != nil {
				return Evidence{}, err
			}
			text := read.article.Blocks[index].Text
			if len(text) > remaining {
				return Evidence{}, ErrLimit
			}
			remaining -= len(text)
			out.Headings = append(out.Headings, text)
		}
		for _, index := range passage.Groups {
			if err := ctx.Err(); err != nil {
				return Evidence{}, err
			}
			text := read.article.Groups[index].Label
			if text == "" {
				continue
			}
			if len(text) > remaining {
				return Evidence{}, ErrLimit
			}
			remaining -= len(text)
			out.Labels = append(out.Labels, text)
		}
	}
	if !found {
		return Evidence{}, ErrMismatch
	}
	if err := ctx.Err(); err != nil {
		return Evidence{}, err
	}
	return out, nil
}

// SearchAny is the explicit any-word/BM25 candidate policy. Search is unchanged.
// Complete union admission precedes output; no article I/O or query retention.
func (r *Reader) SearchAny(ctx context.Context, q termsearch.Query, l termsearch.Limits) (Result, error) {
	if err := ready(ctx); err != nil {
		return Result{}, err
	}
	if r == nil || r.data == nil {
		return Result{}, ErrInvalid
	}
	if q.Edition != r.data.selection.Edition {
		return Result{}, ErrMismatch
	}
	ranked, err := r.data.edition.SearchAny(ctx, q, l)
	if err != nil {
		return Result{}, err
	}
	out := Result{RankingVersion: ranked.RankingVersion, Hits: make([]Hit, len(ranked.Matches))}
	for i, hit := range ranked.Matches {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		out.Hits[i] = Hit{Ref: Reference{owner: r.data, passage: hit.Passage}, Score: hit.Score}
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	return out, nil
}
