// Package htmlsearch binds prepared retained-HTML matches to their exact article
// locations. It borrows unchanged articles and assembles owned quotes on demand;
// it does not acquire sources, persist citations or publish a generation.
package htmlsearch

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/sidsharma96/concordance/internal/htmlarticle"
	"github.com/sidsharma96/concordance/internal/htmlpassage"
	"github.com/sidsharma96/concordance/internal/htmlterms"
	"github.com/sidsharma96/concordance/internal/termsearch"
	"github.com/sidsharma96/concordance/internal/textterms"
)

var (
	ErrInvalid = errors.New("invalid HTML search snapshot input")
	ErrLimit   = errors.New("HTML search snapshot limit exceeded")
)

// Edition is one exact opaque selection key and its ordered retained articles.
// The caller authorizes input and keeps its source/document/revision/generation
// metadata associated with these edition/article positions. An empty edition is
// valid. This is a temporary input representation, not a persisted source schema.
type Edition struct {
	ID       string
	Articles []htmlarticle.Article
}

// Limits bounds all-edition construction counts and logical string bytes. Every
// field, including nested Fields limits, must be positive. MaxInputBytes counts
// edition IDs and every article string occurrence, not HTMLBytes or backing-array
// capacity. Blocks/Groups count structs even if text is empty. Fields applies per
// article; global prepared passages/terms/bytes are charged AFTER one bounded
// article is prepared. Temporary memory can include admitted prior output plus
// one prepared article and its full plan. These are not exact heap/CPU quotas.
type Limits struct {
	Fields         htmlterms.Limits
	MaxEditions    int
	MaxArticles    int
	MaxBlocks      int
	MaxGroups      int
	MaxInputBytes  int
	MaxPassages    int
	MaxTerms       int
	MaxOutputBytes int
}

// Snapshot borrows the edition/article input, including nested block/group arrays,
// and owns its prepared terms and body-location tables. Keep ALL aliases unchanged
// from New through the last dependent use. A Reference keeps the entire state and
// borrowed backing allocations reachable. Reassigning the caller's variable does
// not erase them; Go reachability does not prevent mutation or buffer-pool reuse.
//
// Copies of this wrapper share one state and accept the same references. Separately
// built snapshots have independent state, even with identical input. Read-only
// Search, Location and Quote calls may run concurrently with unchanged input;
// scratch is per-call. No mutable input slices are exposed, goroutines or I/O start,
// or explicit Close is needed. The caller authorizes each use; possession of a
// snapshot/reference is not permission. Nil/zero snapshots are invalid.
type Snapshot struct{ data *state }

type state struct {
	input     []Edition
	corpus    termsearch.Corpus
	locations [][]Location
}

// Reference is an opaque temporary handle to one snapshot state. It cannot be
// serialized as a durable citation or used as authorization. Retaining even one
// handle can retain all snapshot input/terms. Zero and cross-state handles fail.
type Reference struct {
	owner            *state
	edition, passage int
}

// Location owns only values: indices into New's exact input ordering, an
// article-local canonical passage index and its half-open body block range.
// These values are not persistent identities. Editing a Location cannot redirect
// a Reference. The caller preserves source associations and the unchanged article.
type Location struct {
	Edition, Article, Passage int
	Body                      htmlpassage.Range
}

type Hit struct {
	Ref   Reference
	Score float64
}

// Result owns Hits, in the existing RankBM25 order. References retain the snapshot
// state; no body strings or mutable borrowed views are returned. EditionFound
// distinguishes missing from known empty editions. RankingVersion is BM25Version
// on success, including no matches. Every error returns a zero Result.
type Result struct {
	EditionFound   bool
	RankingVersion string
	Hits           []Hit
}

// New admits all input, builds canonical per-article fields once, and retains
// borrowed articles plus owned field/location tables. It accepts no unchecked
// external passage plan. A deadline is required; cancellation is cooperative.
// Every error returns nil, including a malformed late article or global overflow.
// Lower-stage errors wrap with numeric locations, preserving errors.Is without
// captured payloads. This is not publication, authentication or a raw-text query API.
func New(ctx context.Context, input []Edition, limits Limits) (*Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if _, ok := ctx.Deadline(); !ok || !validLimits(limits) {
		return nil, ErrInvalid
	}
	if err := admit(ctx, input, limits); err != nil {
		return nil, err
	}
	if err := editionsValid(ctx, input); err != nil {
		return nil, err
	}
	d := &state{input: input, corpus: termsearch.Corpus{TermsVersion: textterms.Version}, locations: make([][]Location, len(input))}
	d.corpus.Editions = make([]termsearch.Edition, len(input))
	passages, terms, bytes := limits.MaxPassages, limits.MaxTerms, limits.MaxOutputBytes
	for ei, edition := range input {
		d.corpus.Editions[ei].ID = edition.ID
		for ai, article := range edition.Articles {
			prepared, err := htmlterms.Build(ctx, article, limits.Fields)
			if err != nil {
				return nil, fmt.Errorf("prepare HTML edition %d article %d: %w", ei, ai, err)
			}
			if len(prepared.Fields) > passages {
				return nil, ErrLimit
			}
			passages -= len(prepared.Fields)
			for _, fields := range prepared.Fields {
				for _, field := range [3][]string{fields.Body, fields.Headings, fields.Labels} {
					if len(field) > terms {
						return nil, ErrLimit
					}
					terms -= len(field)
					for _, term := range field {
						if err := ctx.Err(); err != nil {
							return nil, err
						}
						if len(term) > bytes {
							return nil, ErrLimit
						}
						bytes -= len(term)
					}
				}
			}
			d.corpus.Editions[ei].Passages = append(d.corpus.Editions[ei].Passages, prepared.Fields...)
			for pi, passage := range prepared.Plan.Passages {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
				d.locations[ei] = append(d.locations[ei], Location{Edition: ei, Article: ai, Passage: pi, Body: passage.Body})
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &Snapshot{data: d}, nil
}

// Search ranks caller-prepared terms under the unchanged RankBM25 contract and
// limits. It still validates ALL prepared input on each call; construction does
// not cache statistics or bypass admission. Mapping M complete matches adds O(M)
// work/space plus O(editions) selection to the existing scan/full-sort costs.
// The caller keeps query input unchanged until return. No quote is assembled.
func (s *Snapshot) Search(ctx context.Context, query termsearch.Query, limits termsearch.Limits) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if s == nil || s.data == nil {
		return Result{}, ErrInvalid
	}
	ranked, err := termsearch.RankBM25(ctx, s.data.corpus, query, limits)
	if err != nil {
		return Result{}, fmt.Errorf("rank HTML snapshot: %w", err)
	}
	result := Result{EditionFound: ranked.EditionFound, RankingVersion: ranked.RankingVersion}
	if len(ranked.Matches) > 0 {
		selected := 0
		for i, edition := range s.data.corpus.Editions {
			if err := ctx.Err(); err != nil {
				return Result{}, err
			}
			if edition.ID == query.Edition {
				selected = i
				break
			}
		}
		result.Hits = make([]Hit, 0, len(ranked.Matches))
		for _, match := range ranked.Matches {
			if err := ctx.Err(); err != nil {
				return Result{}, err
			}
			result.Hits = append(result.Hits, Hit{Ref: Reference{owner: s.data, edition: selected, passage: match.Passage}, Score: match.Score})
		}
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	return result, nil
}

// Location resolves only a reference belonging to this state. It returns owned
// scalar metadata in O(1), not a mutable view or a complete persisted citation.
func (s *Snapshot) Location(ref Reference) (Location, error) {
	if s == nil || s.data == nil || ref.owner != s.data || ref.edition < 0 || ref.edition >= len(s.data.locations) ||
		ref.passage < 0 || ref.passage >= len(s.data.locations[ref.edition]) {
		return Location{}, ErrInvalid
	}
	return s.data.locations[ref.edition][ref.passage], nil
}

// Quote joins exact retained Block.Text in the reference's body range with one
// LF between blocks. It adds no context labels, URLs or normalized terms. Count
// text/separators before allocation; maxBytes must be positive. A deadline is
// required, with cooperative checks per block and before return. Any failure
// returns an empty string, never a truncated quote. Empty retained text can also
// succeed with an empty string; check the error.
//
// The returned string owns assembled storage and can outlive the snapshot. One
// quote costs O(blocks + bytes) work, visiting blocks twice, and O(bytes) output.
// Builder capacity/temporary memory is not an exact heap quota. No I/O starts.
func (s *Snapshot) Quote(ctx context.Context, ref Reference, maxBytes int) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if _, ok := ctx.Deadline(); !ok || maxBytes <= 0 {
		return "", ErrInvalid
	}
	location, err := s.Location(ref)
	if err != nil {
		return "", err
	}
	article := s.data.input[location.Edition].Articles[location.Article]
	blocks := article.Blocks[location.Body.First:location.Body.After]
	remaining := maxBytes
	for i, block := range blocks {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if i > 0 {
			if remaining == 0 {
				return "", ErrLimit
			}
			remaining--
		}
		if len(block.Text) > remaining {
			return "", ErrLimit
		}
		remaining -= len(block.Text)
	}
	var text strings.Builder
	text.Grow(maxBytes - remaining)
	for i, block := range blocks {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if i > 0 {
			text.WriteByte('\n')
		}
		text.WriteString(block.Text)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return text.String(), nil
}
