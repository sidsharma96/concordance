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
func ReadDirectory(ctx context.Context, directory string, l Limits) ([]preparedimport.Edition, error) {
	loaded, err := readCapture(ctx, directory, nil, l)
	if err != nil {
		return nil, err
	}
	return loaded.editions(ctx)
}

// DescriptorRef pins exact local descriptor bytes, not semantic input identity.
type DescriptorRef struct {
	SHA256 [sha256.Size]byte
	Bytes  int64
}

// ReadDirectoryExpected compares the pin against the SAME owned descriptor that
// is decoded. Payload verification and importer inputs also share owned buffers.
// All other ownership, trust and cancellation rules match ReadDirectory.
func ReadDirectoryExpected(ctx context.Context, directory string, expected DescriptorRef, l Limits) ([]preparedimport.Edition, error) {
	loaded, err := readCapture(ctx, directory, &expected, l)
	if err != nil {
		return nil, err
	}
	return loaded.editions(ctx)
}

type loadedCapture struct {
	descriptor  []byte
	description Description
	files       []File
	payloads    map[string][]byte
}

func readCapture(ctx context.Context, directory string, expected *DescriptorRef, l Limits) (out loadedCapture, err error) {
	if err = ready(ctx, l); err != nil {
		return loadedCapture{}, err
	}
	if expected != nil && (expected.Bytes <= 0 || expected.Bytes > int64(l.MaxDescriptorBytes)) {
		return loadedCapture{}, ErrInvalid
	}
	if directory == "" {
		return loadedCapture{}, ErrInvalid
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return loadedCapture{}, fmt.Errorf("open capture directory: %w", err)
	}
	defer func() {
		err = errors.Join(err, root.Close())
		if err != nil {
			out = loadedCapture{}
		}
	}()
	ceiling, exact := int64(l.MaxDescriptorBytes), int64(-1)
	if expected != nil {
		exact = expected.Bytes
	}
	body, err := readRegular(ctx, root, DescriptorName, ceiling, exact)
	if err != nil {
		return loadedCapture{}, fmt.Errorf("read capture description: %w", err)
	}
	if expected != nil && (int64(len(body)) != expected.Bytes || sha256.Sum256(body) != expected.SHA256) {
		return loadedCapture{}, ErrMismatch
	}
	d, err := decode(ctx, body, l)
	if err != nil {
		return loadedCapture{}, err
	}
	files, err := validate(ctx, d, l)
	if err != nil {
		return loadedCapture{}, err
	}
	payloads := make(map[string][]byte, len(files))
	for _, f := range files {
		b, e := readRegular(ctx, root, f.Path, f.Bytes, f.Bytes)
		if e != nil {
			return loadedCapture{}, fmt.Errorf("read captured payload: %w", e)
		}
		if sha256.Sum256(b) != f.SHA256 {
			return loadedCapture{}, ErrMismatch
		}
		payloads[f.Path] = b
	}
	return loadedCapture{descriptor: body, description: d, files: files, payloads: payloads}, nil
}

func (d loadedCapture) editions(ctx context.Context) ([]preparedimport.Edition, error) {
	payloads := d.payloads
	out := make([]preparedimport.Edition, 0, len(d.description.Editions))
	for _, e := range d.description.Editions {
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
	readCeiling := maxBytes
	if expected >= 0 {
		readCeiling = min(readCeiling, expected)
	}
	body, err = io.ReadAll(io.LimitReader(&contextReader{ctx: ctx, r: f}, readCeiling+1))
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
