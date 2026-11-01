// Package searchtarget resolves friendly catalog targets and checks candidate
// default editions before explicit activation. It does not authorize callers,
// choose a latest version, expose a CLI or change an already-open reader.
package searchtarget

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/sidsharma96/concordance/internal/catalog"
	"github.com/sidsharma96/concordance/internal/objectstore"
	"github.com/sidsharma96/concordance/internal/publishedsearch"
)

// Activate checks the complete default reader before attempting a conditional
// target update. The caller authorizes the target/collection and keeps trusted
// routes and clients stable through return. No remote writes, receipt updates or
// long database transaction occur. Admission/concurrent-call limits belong to
// the caller; reader limits bound one check, not whole-process heap.
//
// Every attempt opens manifest/index state, even a retry, so a provider outage
// can prevent retry preflight despite an earlier completed update. ResolveTarget
// can still inspect catalog state separately. An error cannot promise no change.
// No reader/buffers are retained after return; all errors return a zero Target.
func Activate(ctx context.Context, c *catalog.Store, objects *objectstore.Store, update catalog.TargetUpdate, limits publishedsearch.Limits) (catalog.Target, error) {
	if err := ready(ctx); err != nil {
		return catalog.Target{}, err
	}
	if c == nil || objects == nil {
		return catalog.Target{}, catalog.ErrInvalid
	}
	if err := update.Validate(); err != nil {
		return catalog.Target{}, err
	}
	v := update.Value
	_, err := publishedsearch.Open(ctx, c, objects, publishedsearch.Selection{
		Collection: v.Collection, Generation: v.Generation, Edition: v.DefaultEdition,
	}, limits)
	if err != nil {
		return catalog.Target{}, fmt.Errorf("check target generation and default edition: %w", err)
	}
	return c.SetTarget(ctx, update)
}

// Resolve copies one generation/default tuple, using a supplied exact edition in
// place of its default when nonempty. The caller authorizes the target and the
// resolved collection before object access and then calls publishedsearch.Open,
// which checks edition existence. Resolution does not certify an override exists.
// Never resolve again partway through the same search/quote sequence. No buffers
// or catalog are retained; errors return a zero Selection. A deadline is required.
func Resolve(ctx context.Context, c *catalog.Store, name, edition string) (publishedsearch.Selection, error) {
	if err := ready(ctx); err != nil {
		return publishedsearch.Selection{}, err
	}
	if c == nil || len(edition) > 256 || !utf8.ValidString(edition) || strings.ContainsRune(edition, 0) {
		return publishedsearch.Selection{}, catalog.ErrInvalid
	}
	t, err := c.ResolveTarget(ctx, name)
	if err != nil {
		return publishedsearch.Selection{}, err
	}
	if edition == "" {
		edition = t.Value.DefaultEdition
	}
	return publishedsearch.Selection{Collection: t.Value.Collection, Generation: t.Value.Generation, Edition: edition}, nil
}

func ready(ctx context.Context) error {
	if ctx == nil {
		return catalog.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, ok := ctx.Deadline(); !ok {
		return catalog.ErrInvalid
	}
	return nil
}
