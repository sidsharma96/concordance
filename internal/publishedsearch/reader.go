// Package publishedsearch opens one catalog-published generation and exact edition
// for repeated lexical searches and strictly verified quotes. It does not import,
// choose a current generation, authorize callers or cache article bodies.
package publishedsearch

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/sidsharma96/concordance/internal/catalog"
	"github.com/sidsharma96/concordance/internal/generationmanifest"
	"github.com/sidsharma96/concordance/internal/htmlarticlejson"
	"github.com/sidsharma96/concordance/internal/htmledition"
	"github.com/sidsharma96/concordance/internal/htmlpassage"
	"github.com/sidsharma96/concordance/internal/objectstore"
	"github.com/sidsharma96/concordance/internal/termsearch"
)

var (
	ErrInvalid  = errors.New("invalid published search input")
	ErrLimit    = errors.New("published search limit exceeded")
	ErrMismatch = errors.New("published search binding mismatch")
)

// Selection pins one already-authorized collection, published generation and exact
// edition. These are opaque IDs, not a latest alias or authorization credentials.
type Selection struct{ Collection, Generation, Edition string }

// Limits are positive component admission bounds, not whole-process heap limits.
// Opening retains the complete manifest and one edition's decoded index. Encoded
// buffers and component scratch add transient memory. Owners bound simultaneous
// opens/readers/queries; this package adds no global cache or admission queue.
type Limits struct {
	Manifest generationmanifest.Limits
	Edition  htmledition.Limits
	Article  htmlarticlejson.Limits
}

// Reader owns immutable opened metadata/index state. Copies share state and accept
// the same references; separately opened readers do not. Queries may run concurrently
// with independent scratch. New publication never changes an existing reader.
//
// The owner authorizes every use and keeps object client routing/lifetime stable
// through the last dependent Quote. The catalog is borrowed only during Open and
// is not retained. No goroutines, file handles or Close are introduced. Releasing
// all reader/reference aliases permits Go to reclaim state, not remote objects.
// Automatic deletion and permission-revocation handling are not provided here.
type Reader struct{ data *state }

type state struct {
	selection Selection
	manifest  *generationmanifest.Manifest
	edition   *htmledition.Edition
	objects   *objectstore.Store
	article   htmlarticlejson.Limits
}

// Reference is a temporary reader-bound passage handle, not a persisted citation
// or permission. Retaining one keeps the complete reader state/object client
// reachable. Zero and cross-state references are rejected before object I/O.
type Reference struct {
	owner   *state
	passage int
}
type Hit struct {
	Ref   Reference
	Score float64
}

// Result owns Hits in complete BM25 order with stable position ties. There is no
// top-k truncation or eager article read. All errors return a zero Result.
type Result struct {
	RankingVersion string
	Hits           []Hit
}

// Location owns its mutable image-slice storage and copies all other values.
// Changing it cannot redirect a Reference. Body is an exact retained block range;
// Git line ranges are neither required nor inferred. No figure bytes are fetched.
type Location struct {
	Selection Selection
	Document  generationmanifest.Binding
	Body      htmlpassage.Range
}

// Open requires initialized clients from their constructors, trusted stable object
// routing and an authoritative catalog. It checks selection/configuration before
// lookup, collection binding before object I/O, and declared root/index byte caps
// before each GET. Only catalog-published references can enter the reader.
//
// Opening strictly reads the manifest and selected index, not other editions,
// articles or figures. A known-empty edition still needs its declared index;
// missing editions never fall back. Strict reads do not consult receipts, so
// quarantined receipt state does not prevent reading surviving published bytes.
// Deadline/cancellation apply to this call, not the returned reader's lifetime.
// Every failure returns nil; no partial reader escapes.
func Open(ctx context.Context, c *catalog.Store, objects *objectstore.Store, selected Selection, l Limits) (*Reader, error) {
	if err := ready(ctx); err != nil {
		return nil, err
	}
	if c == nil || objects == nil || !identifier(selected.Collection, 256) || !identifier(selected.Generation, 256) || !validString(selected.Edition) {
		return nil, ErrInvalid
	}
	if err := validateLimits(l); err != nil {
		return nil, err
	}
	if objects.Binding().Collection != selected.Collection {
		return nil, ErrMismatch
	}
	for _, s := range []string{selected.Collection, selected.Generation, selected.Edition} {
		if len(s) > l.Manifest.MaxStringBytes {
			return nil, ErrLimit
		}
	}
	if len(selected.Edition) > l.Edition.MaxKeyBytes || len(selected.Edition) > l.Edition.MaxPreparedBytes {
		return nil, ErrLimit
	}
	generation, err := c.LookupGeneration(ctx, selected.Collection, selected.Generation)
	if err != nil {
		return nil, fmt.Errorf("look up published generation: %w", err)
	}
	root := objectstore.Ref{SHA256: generation.ManifestSHA256, Bytes: generation.ManifestBytes}
	if root.Bytes > int64(l.Manifest.MaxEncodedBytes) {
		return nil, ErrLimit
	}
	key, err := objects.Key(root)
	if err != nil {
		return nil, fmt.Errorf("locate published manifest: %w", err)
	}
	if key != generation.ManifestKey {
		return nil, ErrMismatch
	}
	body, err := objects.Read(ctx, root)
	if err != nil {
		return nil, fmt.Errorf("read published manifest: %w", err)
	}
	m, err := generationmanifest.Open(ctx, body, generationmanifest.Ref{SHA256: root.SHA256, Bytes: root.Bytes}, selected.Collection, selected.Generation, l.Manifest)
	if err != nil {
		return nil, fmt.Errorf("open published manifest: %w", err)
	}
	index, err := m.EditionIndex(ctx, selected.Edition)
	if err != nil {
		return nil, fmt.Errorf("select published edition: %w", err)
	}
	if index.Bytes > int64(l.Edition.MaxEncodedBytes) {
		return nil, ErrLimit
	}
	wire, err := objects.Read(ctx, objectstore.Ref{SHA256: index.SHA256, Bytes: index.Bytes})
	if err != nil {
		return nil, fmt.Errorf("read published edition: %w", err)
	}
	e, err := m.OpenEdition(ctx, selected.Edition, wire, l.Edition)
	if err != nil {
		return nil, fmt.Errorf("open published edition: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	own := Selection{strings.Clone(selected.Collection), strings.Clone(selected.Generation), strings.Clone(selected.Edition)}
	return &Reader{data: &state{own, m, e, objects, l.Article}}, nil
}

// Search borrows an already-prepared query unchanged through return. Its edition
// must equal this reader's selection; a different edition is an error, not a search
// of another version. Existing term compatibility, all-keyword matching, complete
// output limits and BM25 statistics apply. No I/O occurs and no query is retained.
func (r *Reader) Search(ctx context.Context, q termsearch.Query, l termsearch.Limits) (Result, error) {
	if err := ready(ctx); err != nil {
		return Result{}, err
	}
	if r == nil || r.data == nil {
		return Result{}, ErrInvalid
	}
	if q.Edition != r.data.selection.Edition {
		return Result{}, ErrMismatch
	}
	ranked, err := r.data.edition.Search(ctx, q, l)
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

// Location resolves only this reader's handle, with no database or object request.
func (r *Reader) Location(ctx context.Context, ref Reference) (Location, error) {
	if err := ready(ctx); err != nil {
		return Location{}, err
	}
	if r == nil || r.data == nil || ref.owner != r.data {
		return Location{}, ErrInvalid
	}
	loc, err := r.data.edition.Location(ref.passage)
	if err != nil {
		return Location{}, err
	}
	doc, err := r.data.manifest.Resolve(ctx, r.data.selection.Edition, loc.Article)
	if err != nil {
		return Location{}, err
	}
	return Location{Selection: r.data.selection, Document: doc, Body: loc.Body}, nil
}

// Quote performs one strict article read on demand, then the existing independent
// manifest and edition checks. It returns owned LF-joined text or an empty string
// on error. Positive maxBytes bounds output, not transferred article bytes; Article
// limits from Open bound the declared input before GET and decoding afterward.
// No article cache, receipt mutation, quarantine, repair or deletion runs here.
func (r *Reader) Quote(ctx context.Context, ref Reference, maxBytes int) (string, error) {
	if maxBytes <= 0 {
		return "", ErrInvalid
	}
	read, err := r.readArticle(ctx, ref)
	if err != nil {
		return "", err
	}
	return r.data.edition.Quote(ctx, ref.passage, read.body, r.data.article, maxBytes)
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
func validString(s string) bool              { return s != "" && utf8.ValidString(s) && strings.IndexByte(s, 0) < 0 }
func identifier(s string, maxBytes int) bool { return len(s) <= maxBytes && validString(s) }
