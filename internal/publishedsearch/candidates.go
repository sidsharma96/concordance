package publishedsearch

import (
	"context"
	"fmt"

	"github.com/sidsharma96/concordance/internal/termsearch"
)

// Candidate keeps the actual reader-bound handle alongside verified owned text.
// Its score is lexical; no model has run. Copying it shares Evidence's slices.
type Candidate struct {
	Ref      Reference
	Score    float64
	Evidence Evidence
}

// CandidateResult owns its mutable candidate slice in unchanged any-word order.
// TotalMatches is the complete union, not the number of articles read. References
// retain their exact reader state/client; copied wrappers share candidate storage.
type CandidateResult struct {
	RankingVersion string
	TotalMatches   int
	Candidates     []Candidate
}

// CandidateLimits includes a per-hit declared article-byte budget for the entire
// selected read plan, charged before its first GET. Repeated articles are charged
// repeatedly because this uncached path reads each hit. MaxTextBytes counts quote,
// heading and notice-label bytes, excluding encoding/separators. Positive values
// bound logical work/output, not whole-process heap, model tokens or all network
// traffic including headers/retries/failed responses.
type CandidateLimits struct {
	Evidence        EvidenceLimits
	MaxArticleBytes int64
	MaxTextBytes    int
}

// SearchAnyEvidenceFirst admits a complete any-word ranking, then checks the
// entire requested prefix's declared read cost before fetching candidate articles.
// Each hit gets one strict read and canonical context; no candidate is skipped or
// shortened. Every error returns a zero result, including failures after earlier
// reads succeed. limit must be positive; it is not the complete-match admission.
//
// For K selected hits and B selected text bytes, extra result state is O(K+B),
// alongside the complete ranking and one article/plan scratch. There are K logical
// article GETs; opening a reader separately needs two. No cache, workers or global
// admission is introduced. The caller bounds simultaneous uses and model encoding.
func (r *Reader) SearchAnyEvidenceFirst(ctx context.Context, q termsearch.Query, search termsearch.Limits, l CandidateLimits, limit int) (CandidateResult, error) {
	if limit <= 0 || !validEvidenceLimits(l.Evidence) || l.MaxArticleBytes <= 0 || l.MaxTextBytes <= 0 {
		return CandidateResult{}, ErrInvalid
	}
	ranked, err := r.SearchAny(ctx, q, search)
	if err != nil {
		return CandidateResult{}, err
	}
	selected := ranked.Hits[:min(limit, len(ranked.Hits))]
	remaining := l.MaxArticleBytes
	for _, hit := range selected {
		location, err := r.Location(ctx, hit.Ref)
		if err != nil {
			return CandidateResult{}, err
		}
		size := location.Document.Article.Bytes
		if size > int64(r.data.article.MaxEncodedBytes) || size > remaining {
			return CandidateResult{}, ErrLimit
		}
		remaining -= size
	}
	out := CandidateResult{RankingVersion: ranked.RankingVersion, TotalMatches: len(ranked.Hits), Candidates: make([]Candidate, 0, len(selected))}
	textLeft := l.MaxTextBytes
	for i, hit := range selected {
		evidence, err := r.Evidence(ctx, hit.Ref, l.Evidence)
		if err != nil {
			return CandidateResult{}, fmt.Errorf("candidate evidence %d: %w", i, err)
		}
		if len(evidence.Quote) > textLeft {
			return CandidateResult{}, ErrLimit
		}
		textLeft -= len(evidence.Quote)
		for _, field := range [][]string{evidence.Headings, evidence.Labels} {
			for _, text := range field {
				if len(text) > textLeft {
					return CandidateResult{}, ErrLimit
				}
				textLeft -= len(text)
			}
		}
		out.Candidates = append(out.Candidates, Candidate{Ref: hit.Ref, Score: hit.Score, Evidence: evidence})
	}
	if err := ctx.Err(); err != nil {
		return CandidateResult{}, err
	}
	return out, nil
}
