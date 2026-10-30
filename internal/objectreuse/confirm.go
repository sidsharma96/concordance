package objectreuse

import (
	"context"
	"errors"
	"sync"

	"github.com/sidsharma96/concordance/internal/catalog"
	"github.com/sidsharma96/concordance/internal/objectstore"
)

// confirm consumes already-admitted, deduplicated positions. Only the worker
// receiving an index writes its proof/error slots. The dispatcher owns jobs;
// Wait establishes completion before any slot or borrowed input is released.
func (s *Store) confirm(ctx context.Context, bodies [][]byte, inputs []int, refs []objectstore.Ref, receipts []catalog.Receipt) ([]objectstore.Verified, error) {
	work, cancel := context.WithCancel(ctx)
	defer cancel()
	jobs := make(chan int)
	proofs := make([]objectstore.Verified, len(refs))
	failures := make([]error, len(refs))
	workers := min(max(1, s.limits.MaxConcurrentObjects), len(refs))
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for i := range jobs {
				if work.Err() != nil {
					return
				}
				if receipts[i].Found() {
					failures[i] = s.objects.Check(work, refs[i], receipts[i].ETag())
				} else {
					proofs[i], failures[i] = s.objects.EnsureVerified(work, bodies[inputs[i]])
				}
				if failures[i] != nil {
					cancel()
					return
				}
			}
		})
	}
dispatch:
	for i := range refs {
		if work.Err() != nil {
			break
		}
		select {
		case jobs <- i:
		case <-work.Done():
			break dispatch
		}
	}
	close(jobs)
	wg.Wait()
	callerErr := ctx.Err()
	hasMaterial := false
	for _, err := range failures {
		if err != nil && !cancellationOnly(err) {
			hasMaterial = true
			break
		}
	}
	for i, err := range failures {
		// A transport can originate cancellation while the caller is live.
		// Suppression may hide peer interruptions, never the only failure.
		if callerErr == nil && hasMaterial && cancellationOnly(err) {
			failures[i] = nil
		}
	}
	if err := errors.Join(append(failures, callerErr)...); err != nil {
		return nil, err
	}
	// Compact proofs only after complete success. Missing validators remain
	// strict-only, and known receipt hits produce no new proof.
	out := proofs[:0]
	for i, proof := range proofs {
		if receipts[i].Found() {
			continue
		}
		if _, err := proof.Evidence(); err == nil {
			out = append(out, proof)
		} else if !errors.Is(err, objectstore.ErrNoValidator) {
			return nil, err
		}
	}
	return out, nil
}

// Drop only interruption-only trees. In particular, a joined uncertain-write
// marker plus cancellation remains intact for reconciliation and errors.Is.
func cancellationOnly(err error) bool {
	if err == context.Canceled {
		return true
	}
	switch e := err.(type) {
	case interface{ Unwrap() []error }:
		children := e.Unwrap()
		if len(children) == 0 {
			return false
		}
		for _, child := range children {
			if !cancellationOnly(child) {
				return false
			}
		}
		return true
	case interface{ Unwrap() error }:
		return cancellationOnly(e.Unwrap())
	default:
		return false
	}
}
