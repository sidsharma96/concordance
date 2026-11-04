package importjob

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"

	"github.com/sidsharma96/concordance/internal/capturefiles"
)

// Prepare exclusively creates jobDirectory under an existing trusted parent. It
// owns a verified copy of prepared HTML/recipes and a generated key/candidate;
// Git inputs remain external. No caller buffer survives return. Success means
// local writes/syncs/closes completed, subject to filesystem semantics. No remote
// side effect, publication, activation, automatic retry or cleanup is performed.
//
// Errors return zero Saved but can leave partial or complete local material,
// including a ready job after final sync/close uncertainty. Preserve the path;
// Open checks the saved bytes rather than inventing another key. Existing paths
// are never overwritten. Cancellation cannot preempt blocked filesystem calls.
func Prepare(ctx context.Context, producerDirectory, jobDirectory string, request Request, l Limits) (Saved, error) {
	return prepare(ctx, producerDirectory, jobDirectory, request, l, nativeOperations())
}
func prepare(ctx context.Context, producerDirectory, jobDirectory string, request Request, l Limits, ops operations) (out Saved, err error) {
	if err := ready(ctx, l); err != nil {
		return Saved{}, err
	}
	if len(producerDirectory) > l.Capture.MaxStringBytes || len(jobDirectory) > l.Capture.MaxStringBytes {
		return Saved{}, ErrLimit
	}
	owned, err := ownRequest(request, l)
	if err != nil {
		return Saved{}, err
	}
	parentPath, leaf, err := jobLocation(ctx, producerDirectory, jobDirectory)
	if err != nil {
		return Saved{}, err
	}
	parent, err := os.OpenRoot(parentPath)
	if err != nil {
		return Saved{}, err
	}
	defer func() {
		err = errors.Join(err, parent.Close())
		if err != nil {
			out = Saved{}
		}
	}()
	var root *os.Root
	defer func() {
		if root != nil {
			err = errors.Join(err, root.Close())
			if err != nil {
				out = Saved{}
			}
		}
	}()
	// Claim only after source verification/descriptor encoding has reached its first
	// write. An invalid source cannot leave an apparently useful empty job.
	writeCapture := func(name string, body []byte) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if root == nil {
			if err := parent.Mkdir(leaf, 0700); err != nil {
				return fmt.Errorf("create new import job: %w", err)
			}
			root, err = parent.OpenRoot(leaf)
			if err != nil {
				return err
			}
			if err := root.Mkdir(captureName, 0700); err != nil {
				return err
			}
		}
		return ops.write(ctx, root, captureName+"/"+name, body)
	}
	copied, err := capturefiles.Copy(ctx, producerDirectory, l.Capture, writeCapture)
	if err != nil {
		return Saved{}, fmt.Errorf("copy prepared input: %w", err)
	}
	if !sameSources(owned.Sources, copied.Sources) {
		return Saved{}, ErrMismatch
	}
	capture, err := root.OpenRoot(captureName)
	if err != nil {
		return Saved{}, err
	}
	syncErr := ops.sync(ctx, capture)
	if err := errors.Join(syncErr, capture.Close()); err != nil {
		return Saved{}, err
	}
	key, err := randomID("import-")
	if err != nil {
		return Saved{}, err
	}
	candidate, err := randomID("generation-")
	if err != nil {
		return Saved{}, err
	}
	saved := Saved{Key: key, CandidateID: candidate, Collection: owned.Collection, Sources: owned.Sources}
	body, err := encodeRecord(ctx, saved, copied, l)
	if err != nil {
		return Saved{}, err
	}
	if err := ops.write(ctx, root, recordName, body); err != nil {
		return Saved{}, err
	}
	// The saved key/capture and their directory entries are synced BEFORE marker
	// installation. The marker is not the only surviving copy of operation identity.
	if err := ops.sync(ctx, root); err != nil {
		return Saved{}, err
	}
	if err := ops.sync(ctx, parent); err != nil {
		return Saved{}, err
	}
	digest := sha256.Sum256(body)
	marker := []byte(readyPrefix + hex.EncodeToString(digest[:]) + "\n")
	if err := ops.write(ctx, root, pendingReady, marker); err != nil {
		return Saved{}, err
	}
	if err := ctx.Err(); err != nil {
		return Saved{}, err
	}
	// Link only our fully written/synced/closed inode. It cannot replace READY.
	if err := ops.link(root, pendingReady, readyName); err != nil {
		return Saved{}, err
	}
	if err := ops.remove(root, pendingReady); err != nil {
		return Saved{}, err
	}
	if err := ops.sync(ctx, root); err != nil {
		return Saved{}, err
	}
	if err := ops.sync(ctx, parent); err != nil {
		return Saved{}, err
	}
	if err := ctx.Err(); err != nil {
		return Saved{}, err
	}
	return saved, nil
}
