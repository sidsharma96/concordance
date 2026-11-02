package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sort"

	"github.com/sidsharma96/concordance/internal/capturefiles"
	"github.com/sidsharma96/concordance/internal/htmlarticle"
	"github.com/sidsharma96/concordance/internal/htmlarticlejson"
	"github.com/sidsharma96/concordance/internal/htmledition"
	"github.com/sidsharma96/concordance/internal/htmlpassage"
	"github.com/sidsharma96/concordance/internal/htmlterms"
	"github.com/sidsharma96/concordance/internal/preparedimport"
	"github.com/sidsharma96/concordance/internal/textterms"
)

func captureLimits() capturefiles.Limits {
	return capturefiles.Limits{MaxDescriptorBytes: 4 << 20, MaxEditions: 8, MaxCaptures: 8, MaxPages: 100, MaxDependencies: 12000, MaxFiles: 108, MaxStringBytes: 8192, MaxValueBytes: 4 << 20, MaxFileBytes: 8 << 20, MaxPayloadBytes: 32 << 20}
}
func extractLimits() htmlarticle.Limits {
	return htmlarticle.Limits{MaxInputBytes: 4 << 20, MaxTokenBytes: 128 << 10, MaxTokens: 200000, MaxNodes: 200000, MaxDepth: 512, MaxBlocks: 20000, MaxGroups: 40000, MaxBlockBytes: 1 << 20, MaxOutputBytes: 8 << 20}
}
func buildLimits() htmledition.BuildLimits {
	a := htmlarticlejson.Limits{MaxEncodedBytes: 8 << 20, MaxBlocks: 20000, MaxGroups: 40000, MaxStringBytes: 1 << 20, MaxValueBytes: 8 << 20, MaxGroupDepth: 512, MaxHTMLBytes: 4 << 20}
	p := htmlterms.Limits{Passages: htmlpassage.Limits{MaxBlocks: 20000, MaxGroups: 40000, MaxInputBytes: 8 << 20, MaxPassages: 20000, MaxBodyBytes: 8192, MaxReferences: 200000, MaxContextBytes: 8 << 20}, Field: textterms.Limits{MaxInputBytes: 64 << 10, MaxTerms: 10000, MaxTermBytes: 64 << 10, MaxOutputBytes: 128 << 10}, MaxViewBytes: 8 << 20, MaxTerms: 200000, MaxOutputBytes: 8 << 20}
	e := htmledition.Limits{MaxEncodedBytes: 8 << 20, MaxArticles: 100, MaxArticleBytes: 8 << 20, MaxBlocks: 100000, MaxPassages: 4000, MaxTerms: 200000, MaxPreparedBytes: 2 << 20, MaxKeyBytes: 2 << 20, MaxTermBytes: 64 << 10, MaxVocabulary: 20000, MaxPostings: 200000}
	return htmledition.BuildLimits{File: e, Article: a, Preparation: p}
}

type openedEdition struct {
	index     *htmledition.Edition
	docs      []document
	bodies    [][]byte
	positions map[location]int
}
type location struct {
	Path         string
	First, After int
}

// Build and preflight have no remote I/O. The caller's capture aliases remain
// stable; encoded bodies and the index are owned only by this opened edition.
func openEdition(ctx context.Context, input preparedimport.Edition, s evaluationSet) (openedEdition, error) {
	type page struct {
		source, revision string
		profile          htmlarticle.Profile
		value            preparedimport.Page
	}
	var pages []page
	for _, c := range input.Captures {
		if c.Revision != s.Pins[input.ID] || c.Profile.ID != s.Processing.Profile {
			return openedEdition{}, fmt.Errorf("capture pin/profile mismatch for edition %q", input.ID)
		}
		for _, p := range c.Pages {
			pages = append(pages, page{c.Source, c.Revision, c.Profile, p})
		}
	}
	sort.Slice(pages, func(i, j int) bool {
		if pages[i].source != pages[j].source {
			return pages[i].source < pages[j].source
		}
		return pages[i].value.Path < pages[j].value.Path
	})
	expected := map[string]document{}
	for _, d := range s.Documents {
		if d.Edition == input.ID {
			expected[d.Path] = d
		}
	}
	if len(pages) != len(expected) {
		return openedEdition{}, fmt.Errorf("document inventory mismatch for edition %q", input.ID)
	}
	o := openedEdition{positions: map[location]int{}}
	l := buildLimits()
	for _, p := range pages {
		d, ok := expected[p.value.Path]
		if !ok || d.Source != p.source || d.Revision != p.revision {
			return openedEdition{}, fmt.Errorf("document binding mismatch for %q", p.value.Path)
		}
		delete(expected, p.value.Path)
		a, err := htmlarticle.Extract(ctx, p.value.HTML, p.profile, extractLimits())
		if err != nil {
			return openedEdition{}, err
		}
		b, err := htmlarticlejson.Encode(ctx, a, l.Article)
		if err != nil {
			return openedEdition{}, err
		}
		if fmt.Sprintf("%x", sha256.Sum256(b)) != d.ArticleSHA256 {
			return openedEdition{}, fmt.Errorf("article hash mismatch for %q", d.Path)
		}
		o.docs = append(o.docs, d)
		o.bodies = append(o.bodies, b)
	}
	var err error
	o.index, err = htmledition.Build(ctx, input.ID, o.bodies, l)
	if err != nil {
		return openedEdition{}, err
	}
	// Locations partition every admitted article. Enumerate through declared
	// article passage counts; an unexpected position fails, never signals EOF.
	position := 0
	for ai := range o.docs {
		a, err := o.index.Article(ai)
		if err != nil {
			return openedEdition{}, err
		}
		for range a.Passages {
			loc, err := o.index.Location(position)
			if err != nil {
				return openedEdition{}, err
			}
			if loc.Article != ai {
				return openedEdition{}, fmt.Errorf("invalid article location")
			}
			key := location{o.docs[loc.Article].Path, loc.Body.First, loc.Body.After}
			o.positions[key] = position
			position++
		}
	}
	return o, nil
}

func verifyReferences(ctx context.Context, o openedEdition, s evaluationSet) error {
	for _, q := range s.Intents {
		for _, a := range q.Answers {
			if a.Edition != o.index.ID() {
				continue
			}
			for _, r := range a.References {
				pos, ok := o.positions[location{r.Path, r.First, r.After}]
				if !ok {
					return fmt.Errorf("reference range mismatch for intent %q edition %q", q.ID, a.Edition)
				}
				loc, err := o.index.Location(pos)
				if err != nil {
					return err
				}
				quote, err := o.index.Quote(ctx, pos, o.bodies[loc.Article], buildLimits().Article, 8<<20)
				if err != nil {
					return err
				}
				if fmt.Sprintf("%x", sha256.Sum256([]byte(quote))) != r.QuoteSHA256 {
					return fmt.Errorf("reference quote mismatch for intent %q edition %q", q.ID, a.Edition)
				}
			}
		}
	}
	return nil
}
