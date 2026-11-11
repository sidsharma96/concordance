package publishedsearch

import (
	"context"
	"fmt"

	"github.com/sidsharma96/concordance/internal/objectstore"
	"github.com/sidsharma96/concordance/internal/termsearch"
)

// GroupedCandidateLimits charges distinct encoded article bytes within one call,
// unlike CandidateLimits' per-hit read plan. All limits must be positive.
// MaxTextBytes still counts every returned quote, heading and notice label.
// These are logical work/output bounds, not model-token, whole-heap or all-wire
// traffic limits. Existing per-read, decoding and passage-plan bounds still apply.
type GroupedCandidateLimits struct {
	Evidence                EvidenceLimits
	MaxDistinctArticleBytes int64
	MaxTextBytes            int
}

type articleGroup struct {
	object    objectstore.Ref
	positions []int // Original selected-hit positions, not passage or document IDs.
}

// ErrTextLimit identifies aggregate quote/context overflow in a grouped result.
// It wraps ErrLimit so callers can retain general admission-error handling.
var ErrTextLimit = fmt.Errorf("candidate text bytes: %w", ErrLimit)

// SearchAnyEvidenceGroupedFirst has the same successful values and complete-match
// admission as SearchAnyEvidenceFirst. It admits a distinct-article read plan
// before its first candidate GET, then reads each group strictly once. Manifest,
// edition, decoding and canonical-context checks still run for every passage.
// Every error returns a zero result; groups cannot skip or shorten candidates.
//
// Groups run sequentially in first-selected-occurrence order and fill their
// original result positions. Only one group's encoded body is held at a time;
// no article state is retained by the reader or returned result. Later calls read
// afresh. A group observes its one verified body, not later changes to storage.
// Failure order may differ from the uncached path. No cache or goroutine is added.
//
// For K selected hits, U distinct articles and B returned text bytes, grouping and
// result state use O(K+U+B) space alongside complete ranking and one article's
// decode/plan scratch. Expected map work is O(K), excluding existing metadata
// lookups and per-hit processing. There are U body GETs, U+2K full article hashes,
// 2K decodes and K passage-plan builds on success. Opening separately needs two
// more GETs. Owners still bound simultaneous calls and encoded/model output.
func (r *Reader) SearchAnyEvidenceGroupedFirst(ctx context.Context, q termsearch.Query, search termsearch.Limits, l GroupedCandidateLimits, limit int) (CandidateResult, error) {
	if limit <= 0 || !validEvidenceLimits(l.Evidence) || l.MaxDistinctArticleBytes <= 0 || l.MaxTextBytes <= 0 {
		return CandidateResult{}, ErrInvalid
	}
	ranked, err := r.SearchAny(ctx, q, search)
	if err != nil {
		return CandidateResult{}, err
	}
	return r.evidenceGroupedFirst(ctx, ranked, l, limit)
}

// SearchEvidenceGroupedFirst retains Search's all-term eligibility and complete
// ranking admission, then verifies the requested prefix using the same scoped
// download sharing as SearchAnyEvidenceGroupedFirst. It changes neither scores
// nor result order. Passing search.MaxMatches selects every admitted match.
// All limits, ownership, request isolation and zero-on-error rules above apply.
func (r *Reader) SearchEvidenceGroupedFirst(ctx context.Context, q termsearch.Query, search termsearch.Limits, l GroupedCandidateLimits, limit int) (CandidateResult, error) {
	if limit <= 0 || !validEvidenceLimits(l.Evidence) || l.MaxDistinctArticleBytes <= 0 || l.MaxTextBytes <= 0 {
		return CandidateResult{}, ErrInvalid
	}
	ranked, err := r.Search(ctx, q, search)
	if err != nil {
		return CandidateResult{}, err
	}
	return r.evidenceGroupedFirst(ctx, ranked, l, limit)
}

func (r *Reader) evidenceGroupedFirst(ctx context.Context, ranked Result, l GroupedCandidateLimits, limit int) (CandidateResult, error) {
	selected := ranked.Hits[:min(limit, len(ranked.Hits))]
	groups, err := r.groupArticleReads(ctx, selected, l.MaxDistinctArticleBytes)
	if err != nil {
		return CandidateResult{}, err
	}
	out := CandidateResult{RankingVersion: ranked.RankingVersion, TotalMatches: len(ranked.Hits), Candidates: make([]Candidate, len(selected))}
	textLeft := l.MaxTextBytes
	for _, group := range groups {
		body, err := r.data.objects.Read(ctx, group.object)
		if err != nil {
			return CandidateResult{}, fmt.Errorf("read candidate article for hit %d: %w", group.positions[0], err)
		}
		for _, i := range group.positions {
			hit := selected[i]
			read, err := r.verifyArticle(ctx, hit.Ref, body)
			if err != nil {
				return CandidateResult{}, fmt.Errorf("verify candidate article for hit %d: %w", i, err)
			}
			evidence, err := r.evidence(ctx, hit.Ref, read, l.Evidence)
			if err != nil {
				return CandidateResult{}, fmt.Errorf("candidate evidence %d: %w", i, err)
			}
			textLeft, err = admitEvidence(evidence, textLeft)
			if err != nil {
				return CandidateResult{}, ErrTextLimit
			}
			out.Candidates[i] = Candidate{Ref: hit.Ref, Score: hit.Score, Evidence: evidence}
		}
	}
	if err := ctx.Err(); err != nil {
		return CandidateResult{}, err
	}
	return out, nil
}

// The complete Ref is scoped to this reader's one stable, already-authorized
// Store. A digest alone is neither a source identity nor permission. Location
// checks every selected logical handle; later verification checks every body
// association even when several slots share the same encoded bytes.
func (r *Reader) groupArticleReads(ctx context.Context, hits []Hit, remaining int64) ([]articleGroup, error) {
	var groups []articleGroup
	seen := make(map[objectstore.Ref]int)
	for i, hit := range hits {
		location, err := r.Location(ctx, hit.Ref)
		if err != nil {
			return nil, err
		}
		article := location.Document.Article
		if article.Bytes > int64(r.data.article.MaxEncodedBytes) {
			return nil, ErrLimit
		}
		object := objectstore.Ref{SHA256: article.SHA256, Bytes: article.Bytes}
		group, found := seen[object]
		if !found {
			if object.Bytes > remaining {
				return nil, ErrLimit
			}
			remaining -= object.Bytes
			group = len(groups)
			seen[object] = group
			groups = append(groups, articleGroup{object: object})
		}
		groups[group].positions = append(groups[group].positions, i)
	}
	return groups, nil
}
