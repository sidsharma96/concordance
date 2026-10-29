package generationpublish

import (
	"context"

	"github.com/sidsharma96/concordance/internal/generationmanifest"
	"github.com/sidsharma96/concordance/internal/objectstore"
)

type prepared struct {
	root      objectstore.Ref
	artifacts []objectstore.Ref
}

func (p *Publisher) preflight(ctx context.Context, collection, id string, manifest []byte, artifacts [][]byte) (prepared, error) {
	if len(artifacts) >= p.limits.MaxObjects {
		return prepared{}, ErrLimit
	}
	left := p.limits.MaxInputBytes
	admit := func(body []byte) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		n := int64(len(body))
		if n > left || n > p.limits.MaxBatchBytes {
			return ErrLimit
		}
		left -= n
		return nil
	}
	if err := admit(manifest); err != nil {
		return prepared{}, err
	}
	for _, body := range artifacts {
		if err := admit(body); err != nil {
			return prepared{}, err
		}
	}
	root, err := p.objects.Reference(manifest)
	if err != nil {
		return prepared{}, err
	}
	m, err := generationmanifest.Open(ctx, manifest, manifestRef(root), collection, id, p.limits.Manifest)
	if err != nil {
		return prepared{}, err
	}
	required, err := m.Objects(ctx)
	if err != nil {
		return prepared{}, err
	}
	if len(required) != len(artifacts) {
		return prepared{}, ErrInvalid
	}
	positions := make(map[generationmanifest.Ref]int, len(artifacts))
	refs := make([]objectstore.Ref, len(artifacts))
	for i, body := range artifacts {
		if err := ctx.Err(); err != nil {
			return prepared{}, err
		}
		r, err := p.objects.Reference(body)
		if err != nil {
			return prepared{}, err
		}
		ref := manifestRef(r)
		if _, duplicate := positions[ref]; duplicate {
			return prepared{}, ErrInvalid
		}
		positions[ref] = i
		refs[i] = r
	}
	for _, ref := range required {
		if err := ctx.Err(); err != nil {
			return prepared{}, err
		}
		if _, ok := positions[ref]; !ok {
			return prepared{}, ErrInvalid
		}
	}
	// One diagnostic clone makes iteration concrete without adding mutable views
	// or a new manifest API. This duplicates admitted metadata, not body buffers.
	d, err := m.Describe(ctx)
	if err != nil {
		return prepared{}, err
	}
	for _, edition := range d.Editions {
		if _, err := m.OpenEdition(ctx, edition.ID, artifacts[positions[edition.Index]], p.limits.Edition); err != nil {
			return prepared{}, err
		}
		for slot, doc := range edition.Documents {
			if _, err := m.VerifyArticle(ctx, edition.ID, slot, artifacts[positions[doc.Article]], p.limits.Article); err != nil {
				return prepared{}, err
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return prepared{}, err
	}
	return prepared{root: root, artifacts: refs}, nil
}

func manifestRef(r objectstore.Ref) generationmanifest.Ref {
	return generationmanifest.Ref{SHA256: r.SHA256, Bytes: r.Bytes}
}

func validLimits(l Limits) bool {
	if l.MaxInputBytes <= 0 || l.MaxBatchBytes <= 0 || l.Manifest.MaxObjectBytes <= 0 || l.Manifest.MaxReferencedBytes <= 0 {
		return false
	}
	for _, n := range []int{
		l.MaxObjects, l.MaxBatchObjects,
		l.Manifest.MaxEncodedBytes, l.Manifest.MaxSources, l.Manifest.MaxEditions,
		l.Manifest.MaxCaptures, l.Manifest.MaxFiles, l.Manifest.MaxDocuments,
		l.Manifest.MaxImages, l.Manifest.MaxSourceFiles, l.Manifest.MaxSourceDocuments,
		l.Manifest.MaxStringBytes, l.Manifest.MaxValueBytes,
		l.Edition.MaxEncodedBytes, l.Edition.MaxArticles, l.Edition.MaxArticleBytes,
		l.Edition.MaxBlocks, l.Edition.MaxPassages, l.Edition.MaxTerms,
		l.Edition.MaxPreparedBytes, l.Edition.MaxKeyBytes, l.Edition.MaxTermBytes,
		l.Edition.MaxVocabulary, l.Edition.MaxPostings,
		l.Article.MaxEncodedBytes, l.Article.MaxBlocks, l.Article.MaxGroups,
		l.Article.MaxStringBytes, l.Article.MaxValueBytes, l.Article.MaxGroupDepth, l.Article.MaxHTMLBytes,
	} {
		if n <= 0 {
			return false
		}
	}
	return true
}
