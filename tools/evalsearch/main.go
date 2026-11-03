// Command evalsearch audits a trusted local capture against fixed known-useful
// references. It has no database, provider client or imports. Optional comparison mode
// changes only diagnostic candidate eligibility, never product search.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/sidsharma96/concordance/internal/capturefiles"
	"github.com/sidsharma96/concordance/internal/htmlarticle"
	"github.com/sidsharma96/concordance/internal/htmlpassage"
	"github.com/sidsharma96/concordance/internal/htmlterms"
	"github.com/sidsharma96/concordance/internal/preparedimport"
	"github.com/sidsharma96/concordance/internal/termsearch"
	"github.com/sidsharma96/concordance/internal/textterms"
)

type hitReport struct {
	Path          string   `json:"path"`
	First         int      `json:"first"`
	After         int      `json:"after"`
	Score         float64  `json:"score"`
	Judgment      string   `json:"judgment"`
	ArticleSHA256 string   `json:"article_sha256,omitempty"`
	Quote         string   `json:"quote,omitempty"`
	Headings      []string `json:"headings,omitempty"`
	Labels        []string `json:"labels,omitempty"`
}
type caseReport struct {
	ID             string      `json:"id"`
	Edition        string      `json:"edition"`
	Form           string      `json:"form"`
	Matches        int         `json:"matches"`
	FirstKnownRank int         `json:"first_known_rank"` // zero means none in the complete ranking
	Top            []hitReport `json:"top_five"`
	DistinctTerms  int         `json:"distinct_terms,omitempty"`
	SingleTermHits int         `json:"summed_single_term_hits,omitempty"`
}
type summary struct {
	Edition     string `json:"edition"`
	Form        string `json:"form"`
	Cases       int    `json:"cases"`
	KnownAtOne  int    `json:"known_at_one"`
	KnownAtFive int    `json:"known_at_five"`
	ZeroMatches int    `json:"zero_matches"`
}
type report struct {
	SetSHA256      string        `json:"set_sha256"`
	CaptureSHA256  string        `json:"capture_sha256"`
	Processing     processing    `json:"processing"`
	RankingVersion string        `json:"ranking_version"`
	Split          string        `json:"split"`
	Documents      int           `json:"documents"`
	References     int           `json:"verified_reference_occurrences"`
	Summaries      []summary     `json:"summaries"`
	Cases          []caseReport  `json:"cases"`
	Controls       []caseReport  `json:"scope_controls"`
	AnyWord        *anyReport    `json:"match_any,omitempty"`
	Noise          []noiseReport `json:"noise_controls,omitempty"`
	Reranked       *rerankReport `json:"reranked,omitempty"`
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "evalsearch:", err)
		os.Exit(1)
	}
}
func run(parent context.Context, args []string, out io.Writer) error {
	f := flag.NewFlagSet("evalsearch", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	directory := f.String("capture", "", "trusted retained capture directory")
	setPath := f.String("set", "", "reviewed evaluation-set JSON; kept outside source history")
	compare := f.Bool("compare-any", false, "compare any-word matching on development only; diagnostic full quotes")
	noise := f.Bool("compare-noise", false, "compare fixed question-word and minimum-half rules on development only")
	export := f.Bool("rerank-export", false, "export first100 candidates; reserved requires explicit validation")
	scores := f.String("rerank-scores", "", "checked local scores; reserved requires explicit validation")
	validateReserved := f.Bool("validate-reserved", false, "explicit frozen validation; requires -split reserved and one rerank mode")
	split := f.String("split", "development", "development, reserved or all; controls accompany development")
	timeout := f.Duration("timeout", 2*time.Minute, "whole audit deadline; local file I/O is not preemptive")
	if err := f.Parse(args); err != nil {
		return err
	}
	if parent == nil || out == nil || f.NArg() != 0 || *directory == "" || *setPath == "" || *timeout <= 0 || (*split != "development" && *split != "reserved" && *split != "all") {
		return errors.New("provide -capture, -set, a valid -split and positive -timeout")
	}
	rerankMode := *export || *scores != ""
	if *validateReserved && (*split != "reserved" || !rerankMode || *compare || *noise) {
		return errors.New("-validate-reserved requires -split reserved and one rerank mode")
	}
	if (*compare || *noise) && *split != "development" {
		return errors.New("comparison flags require the development split")
	}
	if rerankMode && *split != "development" && !*validateReserved {
		return errors.New("reserved reranking requires explicit -validate-reserved")
	}
	if (*export && *scores != "") || ((*export || *scores != "") && (*compare || *noise)) {
		return errors.New("rerank modes are mutually exclusive and cannot combine with comparison flags")
	}
	ctx, cancel := context.WithTimeout(parent, *timeout)
	defer cancel()
	s, hash, err := loadSet(*setPath)
	if err != nil {
		return fmt.Errorf("load set: %w", err)
	}
	if err := compatible(s.Processing); err != nil {
		return err
	}
	descriptor, err := readBounded(filepath.Join(*directory, capturefiles.DescriptorName), 4<<20)
	if err != nil {
		return err
	}
	if fmt.Sprintf("%x", sha256.Sum256(descriptor)) != s.CaptureSHA256 {
		return errors.New("capture descriptor hash mismatch")
	}
	editions, err := capturefiles.ReadDirectory(ctx, *directory, captureLimits())
	if err != nil {
		return err
	}
	if len(editions) != len(s.Pins) {
		return errors.New("edition inventory mismatch")
	}
	sort.Slice(editions, func(i, j int) bool { return editions[i].ID < editions[j].ID })
	seen := map[string]bool{}
	for _, e := range editions {
		if _, ok := s.Pins[e.ID]; !ok || seen[e.ID] {
			return errors.New("unknown or duplicate edition")
		}
		seen[e.ID] = true
		// Complete fixture preflight precedes even development queries. No opened
		// index escapes this phase; query execution deliberately rebuilds it.
		o, err := openEdition(ctx, e, s)
		if err != nil {
			return err
		}
		if err := verifyReferences(ctx, o, s); err != nil {
			return err
		}
	}
	if *export || *scores != "" {
		bundle, err := buildRerankBundle(ctx, editions, s, hash, *split)
		if err != nil {
			return err
		}
		if *export {
			return writeAudit(ctx, out, bundle, 128<<20)
		}
		checked, err := loadRerankScores(ctx, *scores, bundle)
		if err != nil {
			return err
		}
		r, err := evaluate(ctx, editions, s, hash, *split, true, false)
		if err != nil {
			return err
		}
		r.Reranked, err = describeReranked(ctx, bundle, checked, s)
		if err != nil {
			return err
		}
		return writeAudit(ctx, out, r, 16<<20)
	}
	r, err := evaluate(ctx, editions, s, hash, *split, *compare || *noise, *noise)
	if err != nil {
		return err
	}
	return writeAudit(ctx, out, r, 16<<20)
}

func writeAudit(ctx context.Context, out io.Writer, value any, limit int) error {
	body, err := auditBytes(value, limit)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	n, err := out.Write(body)
	if err == nil && n != len(body) {
		err = io.ErrShortWrite
	}
	return err
}
func auditBytes(value any, limit int) ([]byte, error) {
	var body bytes.Buffer
	enc := json.NewEncoder(&body)
	enc.SetIndent("", "  ")
	if err := enc.Encode(value); err != nil {
		return nil, err
	}
	if body.Len() > limit {
		return nil, errors.New("encoded audit byte limit exceeded")
	}
	return body.Bytes(), nil
}

func compatible(p processing) error {
	if p.Article != htmlarticle.Processor || p.Passages != htmlpassage.Processor || p.Fields != htmlterms.Processor || p.Terms != textterms.Version || !text(p.Profile) {
		return errors.New("incompatible evaluation processing rules")
	}
	return nil
}
func queryLimits() termsearch.Limits {
	return termsearch.Limits{MaxEditions: 1, MaxPassages: 4000, MaxTerms: 200000, MaxQueryTerms: 64, MaxInputBytes: 2 << 20, MaxMatches: 4000}
}
func searchPair(ctx context.Context, o openedEdition, id, form, raw string, refs []reference, compare bool) (caseReport, caseReport, error) {
	fail := func(err error) (caseReport, caseReport, error) { return caseReport{}, caseReport{}, err }
	prepared, err := textterms.Prepare(ctx, raw, textterms.Limits{MaxInputBytes: 4096, MaxTerms: 64, MaxTermBytes: 4096, MaxOutputBytes: 4096})
	if err != nil {
		return fail(err)
	}
	if len(prepared.Terms) == 0 {
		return fail(errors.New("query has no technical terms"))
	}
	q := termsearch.Query{Edition: o.index.ID(), TermsVersion: prepared.Version, Terms: prepared.Terms}
	baseline, err := o.index.Search(ctx, q, queryLimits())
	if err != nil {
		return fail(err)
	}
	c, err := describeCase(ctx, o, id, form, refs, baseline)
	if err != nil {
		return fail(err)
	}
	if !compare {
		return c, caseReport{}, nil
	}
	ranked, distinct, hits, err := rankAny(ctx, o, q, baseline)
	if err != nil {
		return fail(err)
	}
	trial, err := describeCase(ctx, o, id, form, refs, ranked)
	if err != nil {
		return fail(err)
	}
	trial.DistinctTerms, trial.SingleTermHits = distinct, hits
	if err := addEvidence(ctx, o, &c); err != nil {
		return fail(err)
	}
	if err := addEvidence(ctx, o, &trial); err != nil {
		return fail(err)
	}
	return c, trial, nil
}
func describeCase(ctx context.Context, o openedEdition, id, form string, refs []reference, ranked termsearch.RankedResult) (caseReport, error) {
	return describeTop(ctx, o, id, form, refs, ranked, 5)
}
func describeTop(ctx context.Context, o openedEdition, id, form string, refs []reference, ranked termsearch.RankedResult, count int) (caseReport, error) {
	known := map[location]bool{}
	for _, r := range refs {
		known[location{r.Path, r.First, r.After}] = true
	}
	r := caseReport{ID: id, Edition: o.index.ID(), Form: form, Matches: len(ranked.Matches), Top: []hitReport{}}
	for i, h := range ranked.Matches {
		if err := ctx.Err(); err != nil {
			return caseReport{}, err
		}
		loc, err := o.index.Location(h.Passage)
		if err != nil {
			return caseReport{}, err
		}
		path := o.docs[loc.Article].Path
		k := location{path, loc.Body.First, loc.Body.After}
		useful := known[k]
		if useful && r.FirstKnownRank == 0 {
			r.FirstKnownRank = i + 1
		}
		if i < count {
			judgment := "unjudged"
			if useful {
				judgment = "known_useful"
			}
			r.Top = append(r.Top, hitReport{Path: path, First: loc.Body.First, After: loc.Body.After, Score: h.Score, Judgment: judgment})
		}
	}
	return r, nil
}
func accumulate(s *summary, c caseReport) {
	s.Cases++
	if c.FirstKnownRank == 1 {
		s.KnownAtOne++
	}
	if c.FirstKnownRank > 0 && c.FirstKnownRank <= 5 {
		s.KnownAtFive++
	}
	if c.Matches == 0 {
		s.ZeroMatches++
	}
}
func evaluate(ctx context.Context, editions []preparedimport.Edition, s evaluationSet, hash, split string, compare, noise bool) (report, error) {
	r := report{SetSHA256: hash, CaptureSHA256: s.CaptureSHA256, Processing: s.Processing, RankingVersion: termsearch.BM25Version, Split: split, Documents: len(s.Documents), Summaries: []summary{}, Cases: []caseReport{}, Controls: []caseReport{}}
	if compare {
		r.AnyWord = &anyReport{Policy: anyPolicy, Summaries: []summary{}, Cases: []caseReport{}, Controls: []caseReport{}}
	}
	if noise {
		r.Noise = []noiseReport{{Policy: questionPolicy, RemovedWords: strings.Fields(questionWords), Summaries: []summary{}, Cases: []noiseCase{}, Controls: []noiseCase{}}, {Policy: halfPolicy, Summaries: []summary{}, Cases: []noiseCase{}, Controls: []noiseCase{}}}
	}
	for _, q := range s.Intents {
		for _, a := range q.Answers {
			r.References += len(a.References)
		}
	}
	for _, input := range editions {
		o, err := openEdition(ctx, input, s)
		if err != nil {
			return report{}, err
		}
		sums := [2]summary{{Edition: input.ID, Form: "keywords"}, {Edition: input.ID, Form: "natural"}}
		anySums := sums
		noiseSums := [2][2]summary{sums, sums}
		for _, q := range s.Intents {
			if split != "all" && q.Split != split {
				continue
			}
			var refs []reference
			for _, a := range q.Answers {
				if a.Edition == input.ID {
					refs = a.References
				}
			}
			for fi, raw := range []string{q.Keywords, q.Natural} {
				c, trial, err := searchPair(ctx, o, q.ID, sums[fi].Form, raw, refs, compare)
				if err != nil {
					return report{}, fmt.Errorf("query %q edition %q: %w", q.ID, input.ID, err)
				}
				r.Cases = append(r.Cases, c)
				accumulate(&sums[fi], c)
				if compare {
					r.AnyWord.Cases = append(r.AnyWord.Cases, trial)
					accumulate(&anySums[fi], trial)
				}
				if noise {
					results, err := searchNoisePair(ctx, o, q.ID, sums[fi].Form, raw, refs)
					if err != nil {
						return report{}, fmt.Errorf("noise query %q edition %q: %w", q.ID, input.ID, err)
					}
					for i, result := range results {
						r.Noise[i].Cases = append(r.Noise[i].Cases, result)
						accumulate(&noiseSums[i][fi], result.caseReport)
					}
				}
			}
		}
		r.Summaries = append(r.Summaries, sums[:]...)
		if compare {
			r.AnyWord.Summaries = append(r.AnyWord.Summaries, anySums[:]...)
		}
		if noise {
			for i := range r.Noise {
				r.Noise[i].Summaries = append(r.Noise[i].Summaries, noiseSums[i][:]...)
			}
		}
		if split != "reserved" {
			for _, q := range s.Controls {
				c, trial, err := searchPair(ctx, o, q.ID, "control", q.Query, nil, compare)
				if err != nil {
					return report{}, err
				}
				r.Controls = append(r.Controls, c)
				if compare {
					r.AnyWord.Controls = append(r.AnyWord.Controls, trial)
				}
				if noise {
					results, err := searchNoisePair(ctx, o, q.ID, "control", q.Query, nil)
					if err != nil {
						return report{}, err
					}
					for i, result := range results {
						r.Noise[i].Controls = append(r.Noise[i].Controls, result)
					}
				}
			}
		}
	}
	if len(r.Cases) == 0 {
		return report{}, errors.New("selected split contains no query intents")
	}
	return r, nil
}
