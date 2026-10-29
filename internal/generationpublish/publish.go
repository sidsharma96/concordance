// Package generationpublish composes bounded artifact checks, immutable object
// confirmation and guarded catalog publication. It is not a complete importer or
// a constructor of trusted captured-input fingerprints.
package generationpublish

import (
	"context"
	"errors"
	"fmt"

	"github.com/sidsharma96/concordance/internal/catalog"
	"github.com/sidsharma96/concordance/internal/generationmanifest"
	"github.com/sidsharma96/concordance/internal/htmlarticlejson"
	"github.com/sidsharma96/concordance/internal/htmledition"
	"github.com/sidsharma96/concordance/internal/objectreuse"
	"github.com/sidsharma96/concordance/internal/objectstore"
)

var (
	ErrInvalid = errors.New("invalid generation publication input")
	ErrLimit   = errors.New("generation publication limit exceeded")
	ErrBusy    = errors.New("generation publisher already has a caller")
)

// Limits are positive logical ceilings, not exact heap or CPU quotas. Objects and
// InputBytes count every supplied artifact plus the manifest. Each must fit a
// batch. Codec metadata, maps, index state, decoded articles and GC add memory.
type Limits struct {
	MaxObjects, MaxBatchObjects  int
	MaxInputBytes, MaxBatchBytes int64
	Manifest                     generationmanifest.Limits
	Edition                      htmledition.Limits
	Article                      htmlarticlejson.Limits
}

// Publisher borrows clients and an already-authorized, captured namespace. Their
// owner configures trusted routing, primary database durability and authorization,
// and closes clients after all calls finish. No input buffers survive Publish.
// One caller per instance is admitted without waiting; other instances/processes
// are not fenced by this permit. Catalog publication still chooses the winner.
type Publisher struct {
	catalog   *catalog.Store
	objects   *objectstore.Store
	reuse     *objectreuse.Store
	namespace catalog.Namespace
	limits    Limits
	permit    chan struct{}
}

func New(c *catalog.Store, o *objectstore.Store, n catalog.Namespace, l Limits) (*Publisher, error) {
	if !validLimits(l) {
		return nil, ErrInvalid
	}
	r, err := objectreuse.New(c, o, n, objectreuse.Limits{MaxObjects: l.MaxBatchObjects, MaxInputBytes: l.MaxBatchBytes})
	if err != nil {
		return nil, fmt.Errorf("configure publication confirmation: %w", err)
	}
	return &Publisher{catalog: c, objects: o, reuse: r, namespace: n, limits: l, permit: make(chan struct{}, 1)}, nil
}

// Publish consumes internal trusted input identity, under catalog.Request's trust
// contract. It MUST NOT be exposed as a public digest-only Import operation. Source
// capture, preparation completeness and the full input fingerprint remain caller
// responsibilities. Hashing this output manifest does not establish input identity.
//
// Keep manifest, artifacts and every alias unchanged through return. Artifacts
// must contain exactly one body for each distinct reference declared by manifest,
// excluding manifest itself. No body is copied or retained after the call. Local
// admission and all index/article binding checks precede catalog.Bind. Remote
// writes follow binding; the manifest is confirmed last, then PublishInNamespace.
//
// A completed binding returns its original result without S3 access, even after
// quarantine. Errors return no partial result; writes or a publication may still
// have committed. Reuse the saved key to discover an uncertain outcome. No retry,
// resume, activation or deletion runs automatically. Cancellation is cooperative.
func (p *Publisher) Publish(ctx context.Context, trusted catalog.Request, candidateID string, manifest []byte, artifacts [][]byte) (catalog.Publication, error) {
	if ctx == nil || p == nil || p.permit == nil || trusted.Collection != p.namespace.Binding().Collection {
		return catalog.Publication{}, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return catalog.Publication{}, err
	}
	if _, ok := ctx.Deadline(); !ok {
		return catalog.Publication{}, ErrInvalid
	}
	select {
	case p.permit <- struct{}{}:
		defer func() { <-p.permit }()
	default:
		return catalog.Publication{}, ErrBusy
	}
	input, err := p.preflight(ctx, trusted.Collection, candidateID, manifest, artifacts)
	if err != nil {
		return catalog.Publication{}, fmt.Errorf("validate publication artifacts: %w", err)
	}
	key, err := p.objects.Key(input.root)
	if err != nil {
		return catalog.Publication{}, err
	}
	record, err := p.catalog.Bind(ctx, trusted)
	if err != nil {
		return catalog.Publication{}, fmt.Errorf("bind publication inputs: %w", err)
	}
	if record.Generation != nil {
		return catalog.Publication{Generation: *record.Generation, Duplicate: true}, nil
	}
	if err := p.confirm(ctx, artifacts, input.artifacts); err != nil {
		return catalog.Publication{}, fmt.Errorf("confirm generation artifacts: %w", err)
	}
	if err := p.confirm(ctx, [][]byte{manifest}, []objectstore.Ref{input.root}); err != nil {
		return catalog.Publication{}, fmt.Errorf("confirm generation manifest: %w", err)
	}
	candidate := catalog.Generation{ID: candidateID, ManifestKey: key, ManifestSHA256: input.root.SHA256, ManifestBytes: input.root.Bytes}
	result, err := p.catalog.PublishInNamespace(ctx, trusted, candidate, p.namespace)
	if err != nil {
		return catalog.Publication{}, fmt.Errorf("publish confirmed generation: %w", err)
	}
	return result, nil
}

func (p *Publisher) confirm(ctx context.Context, bodies [][]byte, expected []objectstore.Ref) error {
	if len(bodies) != len(expected) {
		return ErrInvalid
	}
	for first := 0; first < len(bodies); {
		after, left := first, p.limits.MaxBatchBytes
		for after < len(bodies) && after-first < p.limits.MaxBatchObjects && int64(len(bodies[after])) <= left {
			if err := ctx.Err(); err != nil {
				return err
			}
			left -= int64(len(bodies[after]))
			after++
		}
		// Preflight admits every body against this byte ceiling, so progress is
		// guaranteed with unchanged input. Fail rather than loop if misused.
		if after == first {
			return ErrLimit
		}
		refs, err := p.reuse.Ensure(ctx, bodies[first:after])
		if err != nil {
			return err
		}
		if len(refs) != after-first {
			return ErrInvalid
		}
		for i, ref := range refs {
			if ref != expected[first+i] {
				return ErrInvalid
			}
		}
		first = after
	}
	return ctx.Err()
}
