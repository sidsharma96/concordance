// Package htmledition saves one exact edition's HTML count index and body locations.
// It verifies bytes and structure, not publisher authority or semantic preparation.
// Source/document/revision/generation associations and object I/O belong to callers.
package htmledition

import (
	"context"
	"crypto/sha256"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/sidsharma96/concordance/internal/htmlarticlejson"
	"github.com/sidsharma96/concordance/internal/htmlpassage"
	"github.com/sidsharma96/concordance/internal/htmlterms"
	"github.com/sidsharma96/concordance/internal/termsearch"
)

const Format = "concordance.html-edition/v1"

// Retained compatibility is explicit; do not replace these with newest builder tags.
const articleFormat = "concordance.article/v1"
const passageProcessor = "htmlpassage/v1"
const fieldProcessor = "htmlterms/v1"
const termsVersion = "technical/v1/unicode-17.0.0"

var (
	ErrInvalid     = errors.New("invalid HTML edition")
	ErrLimit       = errors.New("HTML edition limit exceeded")
	ErrMismatch    = errors.New("HTML edition binding mismatch")
	ErrUnsupported = errors.New("unsupported HTML edition version")
)

// Limits are positive aggregate logical ceilings, not exact heap/CPU quotas.
// ArticleBytes/Blocks count every slot, even shared bytes. PreparedBytes counts the
// term version, edition and each prepared term occurrence, not retained article text.
// KeyBytes counts cloned version/edition/distinct-word bytes. Temporary wire records,
// count exports, maps, buffers and slice capacity add memory beyond these counters.
type Limits struct {
	MaxEncodedBytes  int
	MaxArticles      int
	MaxArticleBytes  int
	MaxBlocks        int
	MaxPassages      int
	MaxTerms         int
	MaxPreparedBytes int
	MaxKeyBytes      int
	MaxTermBytes     int
	MaxVocabulary    int
	MaxPostings      int
}
type BuildLimits struct {
	File        Limits
	Article     htmlarticlejson.Limits
	Preparation htmlterms.Limits
}

// Ref describes exact encoded bytes. It is not an identity or permission. Expected
// references must come from independently trusted metadata before using Open.
type Ref struct {
	SHA256 [sha256.Size]byte
	Bytes  int
}
type ArticleRef struct {
	Object   Ref
	Blocks   int
	Passages int
}

// Location is edition-local metadata, not a standalone persisted citation identity.
type Location struct {
	Article int
	Body    htmlpassage.Range
}

// Edition owns immutable counts, descriptors and ranges. Copies share state. No
// caller input is retained and no mutable views escape. Read-only methods may run
// concurrently; per-call output/scratch is owned. No I/O, goroutines or Close.
type Edition struct {
	id        string
	index     *termsearch.Index
	articles  []ArticleRef
	locations []Location
}

func validString(s string) bool { return s != "" && utf8.ValidString(s) && strings.IndexByte(s, 0) < 0 }
func ready(ctx context.Context, l Limits) error {
	if ctx == nil {
		return ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, ok := ctx.Deadline(); !ok {
		return ErrInvalid
	}
	if l.MaxEncodedBytes <= 0 || l.MaxArticles <= 0 || l.MaxArticleBytes <= 0 || l.MaxBlocks <= 0 || l.MaxPassages <= 0 || l.MaxTerms <= 0 || l.MaxPreparedBytes <= 0 || l.MaxKeyBytes <= 0 || l.MaxTermBytes <= 0 || l.MaxVocabulary <= 0 || l.MaxPostings <= 0 {
		return ErrInvalid
	}
	return nil
}
func countLimits(l Limits) termsearch.BuildLimits {
	return termsearch.BuildLimits{MaxEditions: 1, MaxPassages: l.MaxPassages, MaxTerms: l.MaxTerms, MaxInputBytes: l.MaxPreparedBytes, MaxVocabulary: l.MaxVocabulary, MaxPostings: l.MaxPostings, MaxKeyBytes: l.MaxKeyBytes}
}
func articleLimitsValid(l htmlarticlejson.Limits) bool {
	return l.MaxEncodedBytes > 0 && l.MaxBlocks > 0 && l.MaxGroups > 0 && l.MaxStringBytes > 0 && l.MaxValueBytes > 0 && l.MaxGroupDepth > 0 && l.MaxHTMLBytes > 0
}
func countError(err error) error {
	if errors.Is(err, termsearch.ErrLimit) {
		return ErrLimit
	}
	if errors.Is(err, termsearch.ErrInvalid) {
		return ErrInvalid
	}
	return err
}

// Build derives fields and locations from the same canonical preparation of each
// exact article. Article byte totals are admitted before decoding. One complete
// article preparation may precede smaller aggregate field limits; it is bounded by
// Preparation. The complete admitted edition fields coexist with the constructed
// count index until return. Errors return nil; caller aliases stay unchanged.
func Build(ctx context.Context, id string, articles [][]byte, l BuildLimits) (*Edition, error) {
	if err := ready(ctx, l.File); err != nil {
		return nil, err
	}
	if !articleLimitsValid(l.Article) || !preparationLimitsValid(l.Preparation) || !validString(id) {
		return nil, ErrInvalid
	}
	if len(id) > l.File.MaxKeyBytes || len(id) > l.File.MaxPreparedBytes {
		return nil, ErrLimit
	}
	if len(articles) > l.File.MaxArticles {
		return nil, ErrLimit
	}
	remaining := l.File.MaxArticleBytes
	for _, b := range articles {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if len(b) > remaining {
			return nil, ErrLimit
		}
		remaining -= len(b)
	}
	e := &Edition{id: strings.Clone(id), articles: make([]ArticleRef, 0, len(articles)), locations: []Location{}}
	fields := []termsearch.Fields{}
	blocksLeft := l.File.MaxBlocks
	termsLeft := l.File.MaxTerms
	bytesLeft := l.File.MaxPreparedBytes
	for _, s := range []string{id, termsVersion} {
		if len(s) > bytesLeft {
			return nil, ErrLimit
		}
		bytesLeft -= len(s)
	}
	for ai, b := range articles {
		a, err := htmlarticlejson.Decode(ctx, b, l.Article)
		if err != nil {
			return nil, err
		}
		if len(a.Blocks) == 0 {
			return nil, ErrInvalid
		}
		if len(a.Blocks) > blocksLeft {
			return nil, ErrLimit
		}
		blocksLeft -= len(a.Blocks)
		p, err := htmlterms.Build(ctx, a, l.Preparation)
		if err != nil {
			return nil, err
		}
		if p.Plan.Processor != passageProcessor || p.Processor != fieldProcessor || p.TermsVersion != termsVersion || a.Processor != "htmlarticle/v1" {
			return nil, ErrUnsupported
		}
		if len(p.Fields) > l.File.MaxPassages-len(fields) {
			return nil, ErrLimit
		}
		for pi, f := range p.Fields {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			for _, field := range [3][]string{f.Body, f.Headings, f.Labels} {
				if len(field) > termsLeft {
					return nil, ErrLimit
				}
				termsLeft -= len(field)
				for _, word := range field {
					if len(word) > l.File.MaxTermBytes || len(word) > bytesLeft {
						return nil, ErrLimit
					}
					bytesLeft -= len(word)
				}
			}
			fields = append(fields, f)
			e.locations = append(e.locations, Location{ai, p.Plan.Passages[pi].Body})
		}
		e.articles = append(e.articles, ArticleRef{Ref{sha256.Sum256(b), len(b)}, len(a.Blocks), len(p.Fields)})
	}
	x, err := termsearch.NewIndex(ctx, termsearch.Corpus{TermsVersion: termsVersion, Editions: []termsearch.Edition{{ID: id, Passages: fields}}}, countLimits(l.File))
	if err != nil {
		return nil, countError(err)
	}
	e.index = x
	if err := validateLocations(ctx, e, l.File); err != nil {
		return nil, err
	}
	return e, nil
}
func (e *Edition) valid() bool { return e != nil && e.index != nil }
func (e *Edition) ID() string {
	if !e.valid() {
		return ""
	}
	return e.id
}
func (e *Edition) Article(position int) (ArticleRef, error) {
	if !e.valid() || position < 0 || position >= len(e.articles) {
		return ArticleRef{}, ErrInvalid
	}
	return e.articles[position], nil
}
func (e *Edition) Location(position int) (Location, error) {
	if !e.valid() || position < 0 || position >= len(e.locations) {
		return Location{}, ErrInvalid
	}
	return e.locations[position], nil
}

// Search retains the count index's complete match-all/BM25, query ownership and
// bounds. Positions are local to this exact edition; callers pin source metadata.
func (e *Edition) Search(ctx context.Context, q termsearch.Query, l termsearch.Limits) (termsearch.RankedResult, error) {
	if !e.valid() {
		return termsearch.RankedResult{}, ErrInvalid
	}
	return e.index.RankBM25(ctx, q, l)
}

func validateLocations(ctx context.Context, e *Edition, l Limits) error {
	if e == nil || !validString(e.id) {
		return ErrInvalid
	}
	if len(e.articles) > l.MaxArticles || len(e.locations) > l.MaxPassages {
		return ErrLimit
	}
	bytesLeft, blocksLeft := l.MaxArticleBytes, l.MaxBlocks
	position := 0
	for ai, a := range e.articles {
		if err := ctx.Err(); err != nil {
			return err
		}
		if a.Object.Bytes <= 0 || a.Blocks <= 0 || a.Passages <= 0 {
			return ErrInvalid
		}
		if a.Object.Bytes > bytesLeft || a.Blocks > blocksLeft {
			return ErrLimit
		}
		bytesLeft -= a.Object.Bytes
		blocksLeft -= a.Blocks
		if a.Passages > len(e.locations)-position {
			return ErrInvalid
		}
		end := 0
		for range a.Passages {
			if err := ctx.Err(); err != nil {
				return err
			}
			r := e.locations[position]
			if r.Article != ai || r.Body.First != end || r.Body.After <= end || r.Body.After > a.Blocks {
				return ErrInvalid
			}
			end = r.Body.After
			position++
		}
		if end != a.Blocks {
			return ErrInvalid
		}
	}
	if position != len(e.locations) {
		return ErrInvalid
	}
	return ctx.Err()
}

func preparationLimitsValid(l htmlterms.Limits) bool {
	p, f := l.Passages, l.Field
	return p.MaxBlocks > 0 && p.MaxGroups > 0 && p.MaxInputBytes > 0 && p.MaxPassages > 0 && p.MaxBodyBytes > 0 && p.MaxReferences > 0 && p.MaxContextBytes > 0 && f.MaxInputBytes > 0 && f.MaxTerms > 0 && f.MaxTermBytes > 0 && f.MaxOutputBytes > 0 && l.MaxViewBytes > 0 && l.MaxTerms > 0 && l.MaxOutputBytes > 0
}
