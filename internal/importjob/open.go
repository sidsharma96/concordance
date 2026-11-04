package importjob

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/sidsharma96/concordance/internal/capturefiles"
)

// Open reads and validates a complete job into owned metadata/input buffers. It
// checks marker/record pins, closed schema/current policy, the pinned descriptor,
// each payload and exact source-ID membership. It does not check Git, publication,
// authorization or source provenance, and requires no remote routing/credentials.
// The operator protects stable local paths during this call; os.Root is no
// filesystem snapshot or hostile-local-filesystem sandbox. Every error returns
// zero Opened. Marker existence alone never certifies the rest of the directory.
func Open(ctx context.Context, directory string, l Limits) (out Opened, err error) {
	if err := ready(ctx, l); err != nil {
		return Opened{}, err
	}
	if len(directory) > l.Capture.MaxStringBytes {
		return Opened{}, ErrLimit
	}
	if directory == "" {
		return Opened{}, ErrInvalid
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return Opened{}, err
	}
	defer func() {
		err = errors.Join(err, root.Close())
		if err != nil {
			out = Opened{}
		}
	}()
	marker, err := readFile(ctx, root, readyName, len(readyPrefix)+65)
	if err != nil {
		return Opened{}, errors.Join(ErrNotReady, err)
	}
	if len(marker) != len(readyPrefix)+65 || !strings.HasPrefix(string(marker), readyPrefix) || marker[len(marker)-1] != '\n' {
		return Opened{}, ErrNotReady
	}
	pin := string(marker[len(readyPrefix) : len(marker)-1])
	for _, c := range pin {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return Opened{}, ErrNotReady
		}
	}
	body, err := readFile(ctx, root, recordName, l.MaxRecordBytes)
	if err != nil {
		return Opened{}, err
	}
	digest := sha256.Sum256(body)
	if hex.EncodeToString(digest[:]) != pin {
		return Opened{}, ErrMismatch
	}
	saved, expected, err := decodeRecord(ctx, body, l)
	if err != nil {
		return Opened{}, err
	}
	editions, err := capturefiles.ReadDirectoryExpected(ctx, filepath.Join(directory, captureName), expected, l.Capture)
	if err != nil {
		return Opened{}, err
	}
	sourceIDs := []string{}
	seen := map[string]bool{}
	for _, edition := range editions {
		for _, capture := range edition.Captures {
			if !seen[capture.Source] {
				if len(sourceIDs) >= l.MaxSources {
					return Opened{}, ErrLimit
				}
				seen[capture.Source] = true
				sourceIDs = append(sourceIDs, capture.Source)
			}
		}
	}
	if !sameSources(saved.Sources, sourceIDs) {
		return Opened{}, ErrMismatch
	}
	if err := ctx.Err(); err != nil {
		return Opened{}, err
	}
	return Opened{Saved: saved, Editions: editions}, nil
}
