package preparedimport

import (
	"bytes"
	"context"
	"crypto/sha256"
	"slices"
	"sort"
	"strings"

	gm "github.com/sidsharma96/concordance/internal/generationmanifest"
	"github.com/sidsharma96/concordance/internal/generationpublish"
	"github.com/sidsharma96/concordance/internal/gitsource"
	"github.com/sidsharma96/concordance/internal/htmlarticle"
	"github.com/sidsharma96/concordance/internal/htmlarticlejson"
	"github.com/sidsharma96/concordance/internal/htmledition"
	"github.com/sidsharma96/concordance/internal/htmlpassage"
	"github.com/sidsharma96/concordance/internal/htmlterms"
	"github.com/sidsharma96/concordance/internal/textterms"
)

type bundle struct {
	manifest  []byte
	artifacts [][]byte
	rules     generationpublish.CapturedRules
}
type staging struct {
	ctx       context.Context
	limits    Limits
	left      int64
	bodies    [][]byte
	positions map[gm.Ref]int
}

func (s *staging) add(body []byte) (gm.Ref, error) {
	if err := s.ctx.Err(); err != nil {
		return gm.Ref{}, err
	}
	if int64(len(body)) > s.limits.Manifest.MaxObjectBytes {
		return gm.Ref{}, ErrLimit
	}
	r := gm.Ref{SHA256: sha256.Sum256(body), Bytes: int64(len(body))}
	if pos, ok := s.positions[r]; ok {
		// Hash collision is not a supported way to share different byte strings.
		if !bytes.Equal(body, s.bodies[pos]) {
			return gm.Ref{}, ErrInvalid
		}
		return r, nil
	}
	if len(s.bodies) >= s.limits.MaxObjects-1 {
		return gm.Ref{}, ErrLimit
	}
	if err := takeBytes(r.Bytes, &s.left); err != nil {
		return gm.Ref{}, err
	}
	s.positions[r] = len(s.bodies)
	s.bodies = append(s.bodies, body)
	return r, nil
}

func (i *Importer) build(ctx context.Context, id string, editions []Edition) (bundle, error) {
	inputLeft, err := i.admit(ctx, id, editions)
	if err != nil {
		return bundle{}, err
	}
	stage := staging{ctx: ctx, limits: i.limits, left: i.limits.MaxStagedBytes, positions: map[gm.Ref]int{}}
	d := gm.Description{Collection: i.collection, Generation: id}
	sources := map[string]bool{}
	profiles := map[string]htmlarticle.Profile{}
	imagesLeft := i.limits.Manifest.MaxImages
	for _, e := range editions {
		out := gm.Edition{ID: e.ID}
		articles := [][]byte{}
		// Sort private ordinals, never the caller's captures or page buffers.
		order := make([]int, len(e.Captures))
		for n := range order {
			order[n] = n
		}
		sort.Slice(order, func(a, b int) bool { return e.Captures[order[a]].Source < e.Captures[order[b]].Source })
		for _, ci := range order {
			c := &e.Captures[ci]
			source := i.sources[c.Source]
			if !sources[c.Source] {
				d.Sources = append(d.Sources, gm.Source{ID: source.ID, Repository: source.Repository})
				sources[c.Source] = true
			}
			capture, documents, bodies, err := i.capture(ctx, *c, source, len(out.Captures), &stage, &inputLeft, &imagesLeft)
			if err != nil {
				return bundle{}, err
			}
			if len(documents) > 0 {
				p := ownedProfile(c.Profile)
				if previous, ok := profiles[p.ID]; ok && !sameProfile(previous, p) {
					return bundle{}, ErrInvalid
				}
				profiles[p.ID] = p
			}
			out.Captures = append(out.Captures, capture)
			out.Documents = append(out.Documents, documents...)
			articles = append(articles, bodies...)
		}
		index, err := htmledition.Build(ctx, e.ID, articles, i.limits.Edition)
		if err != nil {
			return bundle{}, err
		}
		encoded, err := htmledition.Encode(ctx, index, i.limits.Edition.File)
		if err != nil {
			return bundle{}, err
		}
		out.Index, err = stage.add(encoded)
		if err != nil {
			return bundle{}, err
		}
		d.Editions = append(d.Editions, out)
	}
	encoded, err := gm.Encode(ctx, d, i.limits.Manifest)
	if err != nil {
		return bundle{}, err
	}
	if int64(len(encoded)) > i.limits.Manifest.MaxObjectBytes {
		return bundle{}, ErrLimit
	}
	if err := takeBytes(int64(len(encoded)), &stage.left); err != nil {
		return bundle{}, err
	}
	rules := generationpublish.CapturedRules{MaxHashBytes: i.limits.MaxHashBytes, Input: gm.InputRules{ArticleProcessor: htmlarticle.Processor, PassageProcessor: htmlpassage.Processor, FieldProcessor: htmlterms.Processor, TermsVersion: textterms.Version}}
	for _, p := range profiles {
		rules.Input.Profiles = append(rules.Input.Profiles, p)
	}
	// Stable rule ordering helps diagnostics; input fingerprint also normalizes it.
	slices.SortFunc(rules.Input.Profiles, func(a, b htmlarticle.Profile) int { return strings.Compare(a.ID, b.ID) })
	if err := ctx.Err(); err != nil {
		return bundle{}, err
	}
	return bundle{manifest: encoded, artifacts: stage.bodies, rules: rules}, nil
}

func (i *Importer) capture(ctx context.Context, c Capture, source Source, ordinal int, stage *staging, inputLeft *int64, imagesLeft *int) (gm.Capture, []gm.Document, [][]byte, error) {
	raw, err := gitsource.Capture(ctx, gitsource.Request{Repository: source.LocalPath, Commit: c.Revision, Directory: c.Directory}, i.limits.Git)
	if err != nil {
		return gm.Capture{}, nil, nil, err
	}
	if len(raw.Documents) != len(c.Pages) {
		return gm.Capture{}, nil, nil, ErrMembership
	}
	pages := make(map[string][]byte, len(c.Pages))
	for _, p := range c.Pages {
		pages[p.Path] = p.HTML
	}
	for _, doc := range raw.Documents {
		if _, ok := pages[doc.Path]; !ok {
			return gm.Capture{}, nil, nil, ErrMembership
		}
	}
	dependencies, err := gitsource.CaptureFiles(ctx, gitsource.FilesRequest{Repository: source.LocalPath, Commit: c.Revision, Paths: c.Dependencies}, i.limits.Files)
	if err != nil {
		return gm.Capture{}, nil, nil, err
	}
	slices.SortFunc(raw.Documents, func(a, b gitsource.Document) int { return strings.Compare(a.Path, b.Path) })
	out := gm.Capture{Source: c.Source, Revision: raw.Commit, Directory: raw.Directory, Preparation: c.Preparation, Toolchain: c.Toolchain}
	out.Recipe, err = stage.add(c.Recipe)
	if err != nil {
		return gm.Capture{}, nil, nil, err
	}
	positions := make(map[string]int, len(raw.Documents)+len(dependencies.Files))
	addFile := func(f gitsource.Document) error {
		if err := takeBytes(int64(len(f.Body)), inputLeft); err != nil {
			return err
		}
		r, err := stage.add(f.Body)
		if err != nil {
			return err
		}
		positions[f.Path] = len(out.Files)
		out.Files = append(out.Files, gm.File{Path: f.Path, BlobID: f.BlobID, Object: r})
		return nil
	}
	for _, f := range raw.Documents {
		if err := addFile(f); err != nil {
			return gm.Capture{}, nil, nil, err
		}
	}
	for _, f := range dependencies.Files {
		if err := addFile(f); err != nil {
			return gm.Capture{}, nil, nil, err
		}
	}
	docs := make([]gm.Document, 0, len(raw.Documents))
	bodies := make([][]byte, 0, len(raw.Documents))
	for _, rawDoc := range raw.Documents {
		body := pages[rawDoc.Path]
		article, err := htmlarticle.Extract(ctx, body, c.Profile, i.limits.Extract)
		if err != nil {
			return gm.Capture{}, nil, nil, err
		}
		encoded, err := htmlarticlejson.Encode(ctx, article, i.limits.Edition.Article)
		if err != nil {
			return gm.Capture{}, nil, nil, err
		}
		doc := gm.Document{Capture: ordinal, File: positions[rawDoc.Path], NativeID: rawDoc.Path, Location: rawDoc.Path, Profile: article.Profile}
		doc.HTML, err = stage.add(body)
		if err != nil {
			return gm.Capture{}, nil, nil, err
		}
		doc.Article, err = stage.add(encoded)
		if err != nil {
			return gm.Capture{}, nil, nil, err
		}
		for bi, block := range article.Blocks {
			if block.Kind != htmlarticle.Image {
				continue
			}
			if err := take(1, imagesLeft); err != nil {
				return gm.Capture{}, nil, nil, err
			}
			name, ok := imagePath(block.SourceURL)
			if !ok {
				return gm.Capture{}, nil, nil, gm.ErrUnsupported
			}
			fi, ok := positions[name]
			if !ok {
				return gm.Capture{}, nil, nil, gitsource.ErrMissing
			}
			doc.Images = append(doc.Images, gm.Image{Block: bi, Address: block.SourceURL, File: fi})
		}
		docs = append(docs, doc)
		bodies = append(bodies, encoded)
	}
	return out, docs, bodies, nil
}

// This is the existing Git/HTML v1 manifest mapping, not a general URL resolver.
// No network fetch, URL decoding, path cleaning or guessed page-bundle location.
func imagePath(s string) (string, bool) {
	if !strings.HasPrefix(s, "/images/") || s == "/images/" || strings.ContainsAny(s, "\\%?#:") || !cleanPath(strings.TrimPrefix(s, "/")) {
		return "", false
	}
	return "static" + s, true
}
func ownedProfile(p htmlarticle.Profile) htmlarticle.Profile {
	p.OmitClasses = slices.Clone(p.OmitClasses)
	p.OmitIDs = slices.Clone(p.OmitIDs)
	p.NoticeClasses = slices.Clone(p.NoticeClasses)
	slices.Sort(p.OmitClasses)
	slices.Sort(p.OmitIDs)
	slices.Sort(p.NoticeClasses)
	return p
}
func sameProfile(a, b htmlarticle.Profile) bool {
	return a.ID == b.ID && a.ArticleClass == b.ArticleClass && slices.Equal(a.OmitClasses, b.OmitClasses) && slices.Equal(a.OmitIDs, b.OmitIDs) && slices.Equal(a.NoticeClasses, b.NoticeClasses)
}
