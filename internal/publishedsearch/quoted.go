package publishedsearch

import (
	"context"
	"fmt"

	"github.com/sidsharma96/concordance/internal/termsearch"
)

// QuoteLimits bounds one quote and the sum of returned quote bytes. Both must
// be positive. These are logical text-byte bounds, not encoded-output or heap
// limits. Search limits separately bound the number of hits. One candidate quote
// and the existing article-read/decoding scratch may coexist with the admitted
// batch before an aggregate limit failure.
type QuoteLimits struct {
	MaxQuoteBytes      int
	MaxTotalQuoteBytes int
}

// QuotedHit pairs an unchanged ranked hit with its strictly verified, owned text.
// Ref remains a temporary reader-bound handle; it is not a persisted citation.
type QuotedHit struct {
	Ref   Reference
	Score float64
	Quote string
}

// QuotedResult owns Hits and assembled quote strings in unchanged ranking order.
// TotalMatches counts the complete indexed ranking, not verified article bodies.
// Hits may contain a requested prefix. References retain the
// opened reader state and object client just as Result does. Copying this wrapper
// shares the mutable hit slice; keep aliases unchanged during dependent use.
type QuotedResult struct {
	RankingVersion string
	TotalMatches   int
	Hits           []QuotedHit
}

// SearchQuoted searches once and verifies every quote before returning any hits.
// Every failure returns a zero QuotedResult, including a failure after earlier
// quotes succeeded. It preserves complete ranking, scores and exact selection;
// quote-limit overflow is an error, never truncation or permission to skip a hit.
//
// Each hit invokes the existing strict Quote path sequentially, even when hits
// share article bytes. No cache, goroutine or writer is introduced. A caller may
// render only after success and must bound its own encoded output; writer errors
// can still leave partial bytes. Existing authorization, routing/client lifetime
// and simultaneous-call admission remain caller responsibilities.
func (r *Reader) SearchQuoted(ctx context.Context, q termsearch.Query, searchLimits termsearch.Limits, quoteLimits QuoteLimits) (QuotedResult, error) {
	return r.searchQuoted(ctx, q, searchLimits, quoteLimits, 0)
}

// SearchQuotedFirst ranks the complete admitted match set once, then verifies
// only its first min(limit, TotalMatches) quotes. limit must be positive;
// MaxMatches still rejects an oversized complete ranking before article I/O.
// Unselected article bodies are not read or certified. Scores/order, byte bounds,
// zero-on-error behavior and ownership match SearchQuoted. It does not page or
// reduce full ranking/index work; every selected hit still reads its article.
func (r *Reader) SearchQuotedFirst(ctx context.Context, q termsearch.Query, searchLimits termsearch.Limits, quoteLimits QuoteLimits, limit int) (QuotedResult, error) {
	if limit <= 0 {
		return QuotedResult{}, ErrInvalid
	}
	return r.searchQuoted(ctx, q, searchLimits, quoteLimits, limit)
}

// A zero limit is private to the complete-all wrapper above.
func (r *Reader) searchQuoted(ctx context.Context, q termsearch.Query, searchLimits termsearch.Limits, quoteLimits QuoteLimits, limit int) (QuotedResult, error) {
	if err := ready(ctx); err != nil {
		return QuotedResult{}, err
	}
	if quoteLimits.MaxQuoteBytes <= 0 || quoteLimits.MaxTotalQuoteBytes <= 0 {
		return QuotedResult{}, ErrInvalid
	}
	ranked, err := r.Search(ctx, q, searchLimits)
	if err != nil {
		return QuotedResult{}, err
	}
	selected := len(ranked.Hits)
	if limit > 0 {
		selected = min(selected, limit)
	}
	out := QuotedResult{RankingVersion: ranked.RankingVersion, TotalMatches: len(ranked.Hits), Hits: make([]QuotedHit, 0, selected)}
	total := 0
	for i, hit := range ranked.Hits[:selected] {
		text, err := r.Quote(ctx, hit.Ref, quoteLimits.MaxQuoteBytes)
		if err != nil {
			return QuotedResult{}, fmt.Errorf("quote search hit %d: %w", i, err)
		}
		if len(text) > quoteLimits.MaxTotalQuoteBytes-total {
			return QuotedResult{}, fmt.Errorf("buffer search quotes: %w", ErrLimit)
		}
		total += len(text)
		out.Hits = append(out.Hits, QuotedHit{Ref: hit.Ref, Score: hit.Score, Quote: text})
	}
	if err := ctx.Err(); err != nil {
		return QuotedResult{}, err
	}
	return out, nil
}
