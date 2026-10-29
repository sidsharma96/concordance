package capturefiles

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/sidsharma96/concordance/internal/preparedimport"
)

// ReadDirectory reads capture.json and each referenced regular file once. The
// operator authorizes/protects a stable local directory; the descriptor is trusted
// configuration, not an independently authenticated build receipt. Unreferenced
// directory files are not opened; complete Git membership is checked by the importer.
// No source routes, commands, remote reads or writes come from these files.
//
// Returns caller-owned editions/buffers, nil on any failure. The same payload path
// may share one backing buffer in the result. Keep every alias unchanged through
// ImportPrepared. Digest checks and subsequent import use those SAME buffers, not
// reopened files. Files/root are closed before return, including failure paths.
//
// os.Root confines path traversal on supported native platforms; it does not freeze
// files or prevent mount/device access. Lstat/Stat regular-file checks assume no
// adversarial concurrent filesystem replacement. Context checks cannot preempt
// blocked local I/O. No goroutines, automatic journal, resume or durability writes.
func ReadDirectory(ctx context.Context, directory string, l Limits) (out []preparedimport.Edition, err error) {
	if err = ready(ctx, l); err != nil {
		return nil, err
	}
	if directory == "" {
		return nil, ErrInvalid
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, fmt.Errorf("open capture directory: %w", err)
	}
	defer func() {
		err = errors.Join(err, root.Close())
		if err != nil {
			out = nil
		}
	}()
	body, err := readRegular(ctx, root, DescriptorName, int64(l.MaxDescriptorBytes), -1)
	if err != nil {
		return nil, fmt.Errorf("read capture description: %w", err)
	}
	d, err := decode(ctx, body, l)
	if err != nil {
		return nil, err
	}
	files, err := validate(ctx, d, l)
	if err != nil {
		return nil, err
	}
	payloads := make(map[string][]byte, len(files))
	for _, f := range files {
		b, e := readRegular(ctx, root, f.Path, f.Bytes, f.Bytes)
		if e != nil {
			return nil, fmt.Errorf("read captured payload: %w", e)
		}
		if sha256.Sum256(b) != f.SHA256 {
			return nil, ErrMismatch
		}
		payloads[f.Path] = b
	}
	out = make([]preparedimport.Edition, 0, len(d.Editions))
	for _, e := range d.Editions {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		ed := preparedimport.Edition{ID: e.ID, Captures: []preparedimport.Capture{}}
		for _, c := range e.Captures {
			cap := preparedimport.Capture{Source: c.Source, Revision: c.Revision, Directory: c.Directory, Preparation: c.Preparation, Toolchain: c.Toolchain, Recipe: payloads[c.Recipe.Path], Dependencies: c.Dependencies, Profile: c.Profile, Pages: []preparedimport.Page{}}
			for _, page := range c.Pages {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
				cap.Pages = append(cap.Pages, preparedimport.Page{Path: page.Path, HTML: payloads[page.HTML.Path]})
			}
			ed.Captures = append(ed.Captures, cap)
		}
		out = append(out, ed)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// expected=-1 means admit the descriptor's actual size under maxBytes. File data
// is bounded before reading and one extra byte detects growth beyond the expected
// length. A shrinking file fails the same byte-count check. Stat alone is not proof.
func readRegular(ctx context.Context, root *os.Root, name string, maxBytes, expected int64) (body []byte, err error) {
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	info, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, ErrInvalid
	}
	f, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer func() {
		err = errors.Join(err, f.Close())
		if err != nil {
			body = nil
		}
	}()
	info, err = f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, ErrInvalid
	}
	if info.Size() > maxBytes {
		return nil, ErrLimit
	}
	if expected >= 0 && info.Size() != expected {
		return nil, ErrMismatch
	}
	body, err = io.ReadAll(io.LimitReader(&contextReader{ctx: ctx, r: f}, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maxBytes {
		return nil, ErrLimit
	}
	if expected >= 0 && int64(len(body)) != expected {
		return nil, ErrMismatch
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return body, nil
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}
