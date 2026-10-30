// Package objectreuse composes trusted PostgreSQL receipts with immutable object
// operations. It is not an importer, authorization layer or generation codec.
package objectreuse

import (
	"context"
	"errors"

	"github.com/sidsharma96/concordance/internal/catalog"
	"github.com/sidsharma96/concordance/internal/objectstore"
)

var ErrInvalid = errors.New("invalid object reuse input")

type Limits struct {
	MaxObjects    int
	MaxInputBytes int64
	// MaxConcurrentObjects bounds active object chains per Ensure call. Zero
	// preserves sequential operation; explicit values must be between 1 and 8.
	MaxConcurrentObjects int
}

// Store borrows clients and a captured namespace. Their owners configure durable
// primary database access and stable trusted object routing, authorize every call,
// bound waiting callers and close clients. Methods retain no caller byte buffers.
type Store struct {
	catalog   *catalog.Store
	objects   *objectstore.Store
	namespace catalog.Namespace
	limits    Limits
}

func New(c *catalog.Store, o *objectstore.Store, n catalog.Namespace, l Limits) (*Store, error) {
	if c == nil || o == nil || n.ID() == "" || n.Binding() != o.Binding() || l.MaxObjects <= 0 || l.MaxInputBytes <= 0 || l.MaxConcurrentObjects < 0 || l.MaxConcurrentObjects > 8 {
		return nil, ErrInvalid
	}
	return &Store{c, o, n, l}, nil
}
func deadline(ctx context.Context) error {
	if ctx == nil {
		return ErrInvalid
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	if _, ok := ctx.Deadline(); !ok {
		return ErrInvalid
	}
	return nil
}

// Ensure confirms a bounded batch, preserving caller order and duplicate positions.
// Keep every input alias unchanged until return. Success is NOT publication; callers
// must confirm all generation objects and use catalog.PublishInNamespace. Warm
// distinct objects use HEAD; missing receipts use strict creation/full verification.
// Workers borrow inputs and are all joined before return, even on error. The
// concurrency ceiling is per call; callers still bound simultaneous calls.
func (s *Store) Ensure(ctx context.Context, bodies [][]byte) ([]objectstore.Ref, error) {
	if e := deadline(ctx); e != nil {
		return nil, e
	}
	if len(bodies) > s.limits.MaxObjects {
		return nil, ErrInvalid
	}
	remaining := s.limits.MaxInputBytes
	for _, b := range bodies {
		if int64(len(b)) > remaining {
			return nil, ErrInvalid
		}
		remaining -= int64(len(b))
	}
	out := make([]objectstore.Ref, len(bodies))
	unique := make([]objectstore.Ref, 0, len(bodies))
	positions := make(map[objectstore.Ref]int, len(bodies))
	inputs := make([]int, 0, len(bodies))
	for i, b := range bodies {
		if e := ctx.Err(); e != nil {
			return nil, e
		}
		r, e := s.objects.Reference(b)
		if e != nil {
			return nil, e
		}
		out[i] = r
		if _, ok := positions[r]; !ok {
			positions[r] = len(unique)
			unique = append(unique, r)
			inputs = append(inputs, i)
		}
	}
	receipts, e := s.catalog.LookupReceipts(ctx, s.namespace, unique, s.limits.MaxObjects)
	if e != nil {
		return nil, e
	}
	proofs, e := s.confirm(ctx, bodies, inputs, unique, receipts)
	if e != nil {
		// Peer cancellation must not cancel the quarantine operation. Every worker
		// has joined; the original caller's deadline still governs this attempt.
		return nil, s.failure(ctx, e)
	}
	if len(proofs) > 0 {
		if e = s.catalog.RecordVerified(ctx, s.namespace, proofs, s.limits.MaxObjects); e != nil {
			return nil, e
		}
	}
	if e = s.catalog.CheckNamespace(ctx, s.namespace); e != nil {
		return nil, e
	}
	if e = ctx.Err(); e != nil {
		return nil, e
	}
	return out, nil
}

// Read always verifies full body bytes. A failed quarantine is observable; callers
// must not claim it committed or publish after an error. No background retry runs.
func (s *Store) Read(ctx context.Context, r objectstore.Ref) ([]byte, error) {
	if e := deadline(ctx); e != nil {
		return nil, e
	}
	receipts, e := s.catalog.LookupReceipts(ctx, s.namespace, []objectstore.Ref{r}, 1)
	if e != nil {
		return nil, e
	}
	body, e := s.objects.Read(ctx, r)
	if e != nil {
		if receipts[0].Found() {
			return nil, s.failure(ctx, e)
		}
		return nil, e
	}
	return body, nil
}
func (s *Store) failure(ctx context.Context, e error) error {
	if errors.Is(e, objectstore.ErrChanged) || errors.Is(e, objectstore.ErrIntegrity) || errors.Is(e, objectstore.ErrNotFound) {
		return errors.Join(e, s.catalog.Quarantine(ctx, s.namespace))
	}
	return e
}
