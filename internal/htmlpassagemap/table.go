// Package htmlpassagemap saves bounded HTML passage body locations, bound to exact
// article bytes and prepared search fields. It does not persist an index, identify
// sources, authorize readers or publish a generation.
package htmlpassagemap

import (
	"context"
	"crypto/sha256"
	"errors"

	"github.com/sidsharma96/concordance/internal/htmlarticlejson"
	"github.com/sidsharma96/concordance/internal/htmlpassage"
	"github.com/sidsharma96/concordance/internal/htmlterms"
	"github.com/sidsharma96/concordance/internal/termsearch"
)

const Format = "concordance.html-locations/v1"

// Persisted v1 tags must not change merely because a current builder changes.
const passageVersion = "htmlpassage/v1"
const fieldVersion = "htmlterms/v1"
const termsVersion = "technical/v1/unicode-17.0.0"

var (
	ErrInvalid     = errors.New("invalid HTML passage map")
	ErrLimit       = errors.New("HTML passage map limit exceeded")
	ErrUnsupported = errors.New("unsupported HTML passage map version")
	ErrMismatch    = errors.New("HTML passage map binding mismatch")
)

// Limits are positive logical bounds, not exact heap/CPU quotas. MaxFieldBytes
// counts every term occurrence and the three version strings; MaxTermBytes applies
// to terms only. Encoded rows/JSON tokens and slice capacity add transient memory.
type Limits struct {
	MaxEncodedBytes int
	MaxBlocks       int
	MaxPassages     int
	MaxTerms        int
	MaxTermBytes    int
	MaxFieldBytes   int
}

// CaptureLimits admits article decoding and one complete preparation under their
// own limits before checking the final field/table limits. That temporary may
// exceed the smaller Locations term or passage budgets.
type CaptureLimits struct {
	Locations   Limits
	Article     htmlarticlejson.Limits
	Preparation htmlterms.Limits
}

// OpenLimits bounds article decoding separately from the saved location record.
type OpenLimits struct {
	Locations Limits
	Article   htmlarticlejson.Limits
}

// Fields carries ordered prepared terms, not a product index encoding. Capture
// returns owned slices/strings. Open borrows all aliases unchanged until return and
// retains none. Versions describe how fields were made, not proof of provenance.
type Fields struct {
	PassageProcessor string
	Processor        string
	TermsVersion     string
	Passages         []termsearch.Fields
}

// Table owns immutable body ranges and byte bindings. Only Capture or a checked
// reader creates valid state; the zero value is invalid. Copies share immutable
// ranges. It contains no text or source/document/edition identity.
type Table struct {
	articleHash, fieldsHash [sha256.Size]byte
	articleBytes, blocks    int
	rows                    []htmlpassage.Range
}

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
	if l.MaxEncodedBytes <= 0 || l.MaxBlocks <= 0 || l.MaxPassages <= 0 || l.MaxTerms <= 0 || l.MaxTermBytes <= 0 || l.MaxFieldBytes <= 0 {
		return ErrInvalid
	}
	return nil
}

// Capture decodes exact saved article bytes and runs canonical preparation once.
// The returned fields and table come from that same plan. It borrows articleJSON
// until return; every error returns nil and zero Fields. Limits apply per article.
func Capture(ctx context.Context, articleJSON []byte, l CaptureLimits) (*Table, Fields, error) {
	fail := func(err error) (*Table, Fields, error) { return nil, Fields{}, err }
	if err := ready(ctx, l.Locations); err != nil {
		return fail(err)
	}
	if !articleLimitsValid(l.Article) {
		return fail(ErrInvalid)
	}
	a, err := htmlarticlejson.Decode(ctx, articleJSON, l.Article)
	if err != nil {
		return fail(err)
	}
	if len(a.Blocks) > l.Locations.MaxBlocks {
		return fail(ErrLimit)
	}
	prepared, err := htmlterms.Build(ctx, a, l.Preparation)
	if err != nil {
		return fail(err)
	}
	f := Fields{prepared.Plan.Processor, prepared.Processor, prepared.TermsVersion, prepared.Fields}
	hash, err := fingerprint(ctx, f, l.Locations)
	if err != nil {
		return fail(err)
	}
	if len(prepared.Plan.Passages) != len(f.Passages) {
		return fail(ErrInvalid)
	}
	t := &Table{articleHash: sha256.Sum256(articleJSON), fieldsHash: hash, articleBytes: len(articleJSON), blocks: len(a.Blocks)}
	t.rows = make([]htmlpassage.Range, len(prepared.Plan.Passages))
	for i, p := range prepared.Plan.Passages {
		if err := ctx.Err(); err != nil {
			return fail(err)
		}
		t.rows[i] = p.Body
	}
	if err := validate(ctx, t, l.Locations); err != nil {
		return fail(err)
	}
	return t, f, nil
}

// validate checks the persisted representation, never re-runs the passage builder.
// Full contiguous coverage is a v1 invariant; it is not proof of semantic indexing.
func validate(ctx context.Context, t *Table, l Limits) error {
	if t == nil || t.articleBytes <= 0 || t.blocks <= 0 || len(t.rows) == 0 {
		return ErrInvalid
	}
	if t.blocks > l.MaxBlocks || len(t.rows) > l.MaxPassages {
		return ErrLimit
	}
	end := 0
	for _, r := range t.rows {
		if err := ctx.Err(); err != nil {
			return err
		}
		if r.First != end || r.After <= r.First || r.After > t.blocks {
			return ErrInvalid
		}
		end = r.After
	}
	if end != t.blocks {
		return ErrInvalid
	}
	return ctx.Err()
}

func articleLimitsValid(l htmlarticlejson.Limits) bool {
	return l.MaxEncodedBytes > 0 && l.MaxBlocks > 0 && l.MaxGroups > 0 && l.MaxStringBytes > 0 && l.MaxValueBytes > 0 && l.MaxGroupDepth > 0 && l.MaxHTMLBytes > 0
}
