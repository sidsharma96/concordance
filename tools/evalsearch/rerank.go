package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/sidsharma96/concordance/internal/preparedimport"
	"github.com/sidsharma96/concordance/internal/termsearch"
	"github.com/sidsharma96/concordance/internal/textterms"
)

const (
	rerankInputVersion = "headings-notices-complete-body/v1"
	rerankModel        = "BAAI/bge-reranker-v2-m3"
	rerankRevision     = "953dc6f6f85a1b2dbfca4c34a2796e7dde08d41e"
	rerankWeights      = "d9e3e081faff1eefb84019509b2f5558fd74c1a05a2c7db22f74174fcedb5286"
	rerankTokenizer    = "69564b696052886ed0ac63fa393e928384e0f8caada38c1f4864a9bfbf379c15"
	rerankWindow       = 100
)

type rerankCandidate struct {
	PairID        string   `json:"pair_id"`
	Path          string   `json:"path"`
	First         int      `json:"first"`
	After         int      `json:"after"`
	ArticleSHA256 string   `json:"article_sha256"`
	LexicalScore  float64  `json:"lexical_score"`
	Quote         string   `json:"quote"`
	Headings      []string `json:"headings"`
	Labels        []string `json:"labels"`
	ModelText     string   `json:"model_text"`
}
type rerankInputCase struct {
	ID             string            `json:"id"`
	Edition        string            `json:"edition"`
	Form           string            `json:"form"`
	Query          string            `json:"query"`
	IndexedMatches int               `json:"indexed_matches"`
	Candidates     []rerankCandidate `json:"candidates"`
}
type rerankBundle struct {
	ValidationSplit string            `json:"validation_split,omitempty"`
	Format          string            `json:"format"`
	SetSHA256       string            `json:"set_sha256"`
	CaptureSHA256   string            `json:"capture_sha256"`
	InputVersion    string            `json:"input_version"`
	Model           string            `json:"model"`
	Revision        string            `json:"revision"`
	CandidateLimit  int               `json:"candidate_limit"`
	Cases           []rerankInputCase `json:"cases"`
}

// Build borrows capture aliases unchanged synchronously. It owns returned slice
// storage and strings assembled by Quote/modelText. No label is exported. Memory
// includes all input buffers, one edition, full ranking and the accumulated bundle.
func buildRerankBundle(ctx context.Context, inputs []preparedimport.Edition, s evaluationSet, setHash, split string) (rerankBundle, error) {
	if split != "development" && split != "reserved" {
		return rerankBundle{}, errors.New("invalid rerank split")
	}
	b := rerankBundle{Format: "concordance.rerank-input/v1", SetSHA256: setHash, CaptureSHA256: s.CaptureSHA256, InputVersion: rerankInputVersion, Model: rerankModel, Revision: rerankRevision, CandidateLimit: rerankWindow, Cases: []rerankInputCase{}}
	if split == "reserved" {
		b.ValidationSplit = split
	}
	for _, input := range inputs {
		o, err := openEdition(ctx, input, s)
		if err != nil {
			return rerankBundle{}, err
		}
		for _, q := range s.Intents {
			if q.Split != split {
				continue
			}
			for fi, raw := range []string{q.Keywords, q.Natural} {
				form := []string{"keywords", "natural"}[fi]
				c, err := exportRerankCase(ctx, o, q.ID, form, raw)
				if err != nil {
					return rerankBundle{}, err
				}
				b.Cases = append(b.Cases, c)
			}
		}
		if split == "development" {
			for _, q := range s.Controls {
				c, err := exportRerankCase(ctx, o, q.ID, "control", q.Query)
				if err != nil {
					return rerankBundle{}, err
				}
				b.Cases = append(b.Cases, c)
			}
		}
	}
	return b, ctx.Err()
}
func exportRerankCase(ctx context.Context, o openedEdition, id, form, raw string) (rerankInputCase, error) {
	p, err := textterms.Prepare(ctx, raw, textterms.Limits{MaxInputBytes: 4096, MaxTerms: 64, MaxTermBytes: 4096, MaxOutputBytes: 4096})
	if err != nil {
		return rerankInputCase{}, err
	}
	q := termsearch.Query{Edition: o.index.ID(), TermsVersion: p.Version, Terms: p.Terms}
	all, err := o.index.Search(ctx, q, queryLimits())
	if err != nil {
		return rerankInputCase{}, err
	}
	any, _, _, err := rankAny(ctx, o, q, all)
	if err != nil {
		return rerankInputCase{}, err
	}
	c, err := describeTop(ctx, o, id, form, nil, any, rerankWindow)
	if err != nil {
		return rerankInputCase{}, err
	}
	if err := addEvidence(ctx, o, &c); err != nil {
		return rerankInputCase{}, err
	}
	out := rerankInputCase{ID: id, Edition: o.index.ID(), Form: form, Query: raw, IndexedMatches: c.Matches, Candidates: []rerankCandidate{}}
	for _, h := range c.Top {
		candidate := rerankCandidate{Path: h.Path, First: h.First, After: h.After, ArticleSHA256: h.ArticleSHA256, LexicalScore: h.Score, Quote: h.Quote, Headings: h.Headings, Labels: h.Labels, ModelText: modelText(h)}
		body, err := json.Marshal(struct {
			Version, ID, Edition, Form, Query string
			Candidate                         rerankCandidate
		}{rerankInputVersion, id, out.Edition, form, raw, candidate})
		if err != nil {
			return rerankInputCase{}, err
		}
		candidate.PairID = fmt.Sprintf("%x", sha256.Sum256(body))
		out.Candidates = append(out.Candidates, candidate)
	}
	return out, nil
}
func modelText(h hitReport) string {
	var b strings.Builder
	for _, s := range h.Headings {
		b.WriteString("Heading: ")
		b.WriteString(s)
		b.WriteByte('\n')
	}
	for _, s := range h.Labels {
		b.WriteString("Notice: ")
		b.WriteString(s)
		b.WriteByte('\n')
	}
	b.WriteString("Body:\n")
	b.WriteString(h.Quote)
	return b.String()
}

type rerankedHit struct {
	hitReport
	LexicalRank  int     `json:"lexical_rank"`
	LexicalScore float64 `json:"lexical_score"`
}
type rerankedCase struct {
	ID                     string        `json:"id"`
	Edition                string        `json:"edition"`
	Form                   string        `json:"form"`
	IndexedMatches         int           `json:"indexed_matches"`
	Candidates             int           `json:"candidates"`
	FirstKnownRankInWindow int           `json:"first_known_rank_in_window"`
	Top                    []rerankedHit `json:"top_five"`
}
type rerankReport struct {
	ValidationSplit string         `json:"validation_split,omitempty"`
	Policy          string         `json:"policy"`
	BundleSHA256    string         `json:"bundle_sha256"`
	InputVersion    string         `json:"input_version"`
	CandidateLimit  int            `json:"candidate_limit"`
	Runtime         rerankRuntime  `json:"runtime"`
	Summaries       []summary      `json:"summaries"`
	Cases           []rerankedCase `json:"cases"`
	Controls        []rerankedCase `json:"scope_controls"`
}

// A complete score set is checked before entry. Model sorting changes only the
// candidate order; addresses and owned quote/context remain from the exact bundle.
func describeReranked(ctx context.Context, b rerankBundle, scores rerankScores, s evaluationSet) (*rerankReport, error) {
	values := map[string]float64{}
	for _, row := range scores.Entries {
		values[row.PairID] = *row.Score
	}
	r := &rerankReport{ValidationSplit: b.ValidationSplit, Policy: "bge-v2-m3/lexical100/v1", BundleSHA256: scores.BundleSHA256, InputVersion: b.InputVersion, CandidateLimit: b.CandidateLimit, Runtime: scores.Runtime, Summaries: []summary{}, Cases: []rerankedCase{}, Controls: []rerankedCase{}}
	for _, c := range b.Cases {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		order := make([]int, len(c.Candidates))
		for i := range order {
			order[i] = i
		}
		slices.SortStableFunc(order, func(i, j int) int {
			a, z := values[c.Candidates[i].PairID], values[c.Candidates[j].PairID]
			if a > z {
				return -1
			}
			if a < z {
				return 1
			}
			return 0
		})
		known := map[location]bool{}
		for _, q := range s.Intents {
			if q.ID != c.ID {
				continue
			}
			for _, a := range q.Answers {
				if a.Edition != c.Edition {
					continue
				}
				for _, ref := range a.References {
					known[location{ref.Path, ref.First, ref.After}] = true
				}
			}
		}
		result := rerankedCase{ID: c.ID, Edition: c.Edition, Form: c.Form, IndexedMatches: c.IndexedMatches, Candidates: len(order), Top: []rerankedHit{}}
		for rank, pos := range order {
			h := c.Candidates[pos]
			useful := known[location{h.Path, h.First, h.After}]
			if useful && result.FirstKnownRankInWindow == 0 {
				result.FirstKnownRankInWindow = rank + 1
			}
			if rank < 5 {
				judgment := "unjudged"
				if useful {
					judgment = "known_useful"
				}
				result.Top = append(result.Top, rerankedHit{hitReport: hitReport{Path: h.Path, First: h.First, After: h.After, Score: values[h.PairID], Judgment: judgment, ArticleSHA256: h.ArticleSHA256, Quote: h.Quote, Headings: h.Headings, Labels: h.Labels}, LexicalRank: pos + 1, LexicalScore: h.LexicalScore})
			}
		}
		if c.Form == "control" {
			r.Controls = append(r.Controls, result)
			continue
		}
		r.Cases = append(r.Cases, result)
		index := -1
		for i := range r.Summaries {
			if r.Summaries[i].Edition == c.Edition && r.Summaries[i].Form == c.Form {
				index = i
				break
			}
		}
		if index < 0 {
			r.Summaries = append(r.Summaries, summary{Edition: c.Edition, Form: c.Form})
			index = len(r.Summaries) - 1
		}
		accumulate(&r.Summaries[index], caseReport{Matches: len(order), FirstKnownRank: result.FirstKnownRankInWindow})
	}
	return r, nil
}
