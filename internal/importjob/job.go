// Package importjob retains bounded local Git/HTML retry material. It copies
// prepared HTML/recipes, not Git, and never contacts a catalog, store or renderer.
package importjob

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/sidsharma96/concordance/internal/capturefiles"
	"github.com/sidsharma96/concordance/internal/preparedimport"
)

const Format = "concordance.git-html-import-job/v1"
const recordName = "request.json"
const readyName = "READY"
const readyPrefix = "concordance.import-job-ready/v1\n"
const captureName = "capture"
const pendingReady = ".ready"

var (
	ErrInvalid  = errors.New("invalid local import job")
	ErrLimit    = errors.New("local import job limit exceeded")
	ErrNotReady = errors.New("local import job is not ready")
	ErrMismatch = errors.New("local import job bytes or source identities changed")
	ErrPolicy   = errors.New("local import job processing policy is incompatible")
)

// Source contains nonsecret observed identity, never a clone route or credential.
// Caller-owned source registration must match these values before actual import.
type Source struct{ ID, Repository string }

// Request supplies trusted collection/source identity. It is not authorization.
// Keep every alias unchanged through Prepare; no route or credential is retained.
type Request struct {
	Collection string
	Sources    []Source
}

// Capture.MaxStringBytes also bounds each supplied directory path.
type Limits struct {
	Capture                    capturefiles.Limits
	MaxRecordBytes, MaxSources int
}

// Saved owns its metadata; wrapper copies share the mutable Sources slice. It is
// an immutable-on-disk retry description, not a published generation or authority.
// CandidateID is a proposal; the catalog's returned generation wins on retry.
type Saved struct {
	Key, CandidateID, Collection string
	Sources                      []Source
}

// Opened owns checked input buffers and metadata; wrapper copies share slices.
// Keep all aliases unchanged during ImportPrepared. Separate Opens share no state.
// Sources only name expected identities; the owner supplies trusted local routes.
type Opened struct {
	Saved
	Editions []preparedimport.Edition
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
	maxInt := int(^uint(0) >> 1)
	if l.MaxRecordBytes <= 0 || l.MaxRecordBytes == maxInt || l.MaxSources <= 0 {
		return ErrInvalid
	}
	return l.Capture.Validate()
}

func text(s string) bool {
	return s != "" && utf8.ValidString(s) && strings.IndexFunc(s, unicode.IsControl) < 0
}
func identifier(s string) bool { return len(s) <= 256 && text(s) }

func ownRequest(request Request, l Limits) (Request, error) {
	if !identifier(request.Collection) {
		return Request{}, ErrInvalid
	}
	if len(request.Sources) > l.MaxSources {
		return Request{}, ErrLimit
	}
	remaining := l.MaxRecordBytes
	take := func(s string) bool {
		if len(s) > l.Capture.MaxStringBytes || len(s) > remaining {
			return false
		}
		remaining -= len(s)
		return true
	}
	if !take(request.Collection) {
		return Request{}, ErrLimit
	}
	out := Request{Collection: strings.Clone(request.Collection), Sources: make([]Source, len(request.Sources))}
	for i, source := range request.Sources {
		if !identifier(source.ID) || !text(source.Repository) {
			return Request{}, ErrInvalid
		}
		if !take(source.ID) || !take(source.Repository) {
			return Request{}, ErrLimit
		}
		out.Sources[i] = Source{ID: strings.Clone(source.ID), Repository: strings.Clone(source.Repository)}
	}
	slices.SortFunc(out.Sources, func(a, b Source) int { return strings.Compare(a.ID, b.ID) })
	for i := 1; i < len(out.Sources); i++ {
		if out.Sources[i-1].ID == out.Sources[i].ID {
			return Request{}, ErrInvalid
		}
	}
	return out, nil
}
func sameSources(expected []Source, observed []string) bool {
	if len(expected) != len(observed) {
		return false
	}
	set := make(map[string]bool, len(expected))
	for _, s := range expected {
		set[s.ID] = true
	}
	for _, id := range observed {
		if !set[id] {
			return false
		}
		delete(set, id)
	}
	return len(set) == 0
}
func randomID(prefix string) (string, error) {
	var body [16]byte
	if _, err := rand.Read(body[:]); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(body[:]), nil
}
func validID(s, prefix string) bool {
	if !strings.HasPrefix(s, prefix) || len(s) != len(prefix)+32 {
		return false
	}
	for _, c := range s[len(prefix):] {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
