// Package preparedimport imports trusted Git/HTML captures. It owns complete Git
// membership, processing and publication, but does not run a website builder or
// prove the preparer's HTML provenance or rendering-dependency completeness.
package preparedimport

import (
	"context"
	"errors"
	"fmt"

	"github.com/sidsharma96/concordance/internal/catalog"
	gm "github.com/sidsharma96/concordance/internal/generationmanifest"
	"github.com/sidsharma96/concordance/internal/generationpublish"
	"github.com/sidsharma96/concordance/internal/gitsource"
	"github.com/sidsharma96/concordance/internal/htmlarticle"
	"github.com/sidsharma96/concordance/internal/htmledition"
)

var (
	ErrInvalid    = errors.New("invalid prepared import")
	ErrLimit      = errors.New("prepared import limit exceeded")
	ErrMembership = errors.New("prepared pages do not match complete Git selection")
	ErrBusy       = errors.New("prepared importer already has a caller")
)

// Source is trusted registration/routing, not caller-provided authorization.
// Repository is the observed stable source identity; LocalPath routes Git reads
// and is excluded from saved-key input comparison. Keep local Git objects available.
type Source struct{ ID, Repository, LocalPath string }

// Edition selects its complete intended membership, not a patch. A capture can
// select an empty Git directory; a declared edition with no captures is also empty.
type Edition struct {
	ID       string
	Captures []Capture
}

// Capture is an assertion from a trusted preparer. Recipe must record its producer,
// build configuration and output exclusions (e.g. generated print duplicates).
// It is retained verbatim, never executed. Dependencies names every required Git
// build input outside selected Markdown, including referenced original images.
// External tool/dependency identities belong in Toolchain/Recipe. Concordance
// verifies requested Git bytes, not that these declarations describe the build.
// Profile is the actual extraction policy, not a caller-supplied digest.
type Capture struct {
	Source, Revision, Directory, Preparation, Toolchain string
	Recipe                                              []byte
	Dependencies                                        []string
	Profile                                             htmlarticle.Profile
	Pages                                               []Page
}

// Page associates one source Markdown path with captured HTML, not an output-tree
// filename. Every selected Markdown file needs exactly one page. Page ordering is
// irrelevant; logical slots sort by source ID, then full source path per edition.
type Page struct {
	Path string
	HTML []byte
}

// Limits combine aggregate logical ceilings with component budgets. InputBytes
// counts every HTML/recipe occurrence and acquired raw file occurrence, even if
// bytes are shared. StagedBytes counts distinct staged object bodies plus the root.
// A bounded component operation may finish before a smaller aggregate output limit
// rejects it. Caller buffers, Git internals, parser/index scratch, metadata and GC
// add memory; these are not whole-process heap or CPU quotas. All fields are positive.
type Limits struct {
	MaxInputBytes, MaxStagedBytes int64
	MaxObjects, MaxHashBytes      int
	Manifest                      gm.Limits
	Git                           gitsource.Limits
	Files                         gitsource.FileLimits
	Extract                       htmlarticle.Limits
	Edition                       htmledition.BuildLimits
}

// Importer owns copied source registration and borrows a concrete publisher. Its
// owner authorizes the collection/routing and closes clients after calls finish.
// One nonwaiting caller per instance is admitted; this is not a distributed lease.
// No goroutines, builder processes, automatic retries, resume or deletion are added.
type Importer struct {
	publisher  *generationpublish.Publisher
	collection string
	sources    map[string]Source
	limits     Limits
	permit     chan struct{}
}

func New(p *generationpublish.Publisher, collection string, sources []Source, l Limits) (*Importer, error) {
	if p == nil || !identifier(collection) || !validLimits(l) || len(sources) > l.Manifest.MaxSources {
		return nil, ErrInvalid
	}
	remaining := l.Manifest.MaxValueBytes
	for _, s := range sources {
		for _, v := range []string{s.ID, s.Repository, s.LocalPath} {
			if err := admitText(v, l.Manifest.MaxStringBytes, &remaining); err != nil {
				return nil, err
			}
		}
		if !identifier(s.ID) || s.Repository == "" || s.LocalPath == "" {
			return nil, ErrInvalid
		}
	}
	own := make(map[string]Source, len(sources))
	for _, s := range sources {
		if _, ok := own[s.ID]; ok {
			return nil, ErrInvalid
		}
		own[s.ID] = s
	}
	return &Importer{publisher: p, collection: collection, sources: own, limits: l, permit: make(chan struct{}, 1)}, nil
}

// ImportPrepared borrows every request/body/profile alias unchanged through return.
// It reads pinned local Git objects, requires exact complete page membership,
// captures declared dependencies, derives articles/indexes/manifest, then calls
// PublishCaptured. All local construction precedes catalog binding or object I/O.
// No request buffers are retained after return; generated/acquired bytes are local.
//
// Success returns a complete published generation, usable by existing internal
// readers (there is no search CLI). Errors return zero but publication may already
// have committed; retain the exact handoff and retry the saved key to resolve an
// uncertain outcome. A completed retry rebuilds/preflights locally, then returns
// its original result without S3 access. Cancellation is cooperative, not rollback.
func (i *Importer) ImportPrepared(ctx context.Context, key, candidateID string, editions []Edition) (catalog.Publication, error) {
	if i == nil || i.permit == nil || !identifier(key) {
		return catalog.Publication{}, ErrInvalid
	}
	if err := ready(ctx); err != nil {
		return catalog.Publication{}, err
	}
	select {
	case i.permit <- struct{}{}:
		defer func() { <-i.permit }()
	default:
		return catalog.Publication{}, ErrBusy
	}
	b, err := i.build(ctx, candidateID, editions)
	if err != nil {
		return catalog.Publication{}, fmt.Errorf("prepare complete import: %w", err)
	}
	out, err := i.publisher.PublishCaptured(ctx, key, candidateID, b.manifest, b.artifacts, b.rules)
	if err != nil {
		return catalog.Publication{}, fmt.Errorf("publish prepared import: %w", err)
	}
	return out, nil
}
