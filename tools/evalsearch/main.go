// Command evalsearch audits a trusted local capture against fixed known-useful
// references. It has no database, provider client, imports or ranking policy.
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
	Path     string  `json:"path"`
	First    int     `json:"first"`
	After    int     `json:"after"`
	Score    float64 `json:"score"`
	Judgment string  `json:"judgment"`
}
type caseReport struct {
	ID             string      `json:"id"`
	Edition        string      `json:"edition"`
	Form           string      `json:"form"`
	Matches        int         `json:"matches"`
	FirstKnownRank int         `json:"first_known_rank"` // zero means none in the complete ranking
	Top            []hitReport `json:"top_five"`
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
	SetSHA256      string       `json:"set_sha256"`
	CaptureSHA256  string       `json:"capture_sha256"`
	Processing     processing   `json:"processing"`
	RankingVersion string       `json:"ranking_version"`
	Split          string       `json:"split"`
	Documents      int          `json:"documents"`
	References     int          `json:"verified_reference_occurrences"`
	Summaries      []summary    `json:"summaries"`
	Cases          []caseReport `json:"cases"`
	Controls       []caseReport `json:"scope_controls"`
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
	split := f.String("split", "development", "development, reserved or all; controls accompany development")
	timeout := f.Duration("timeout", 2*time.Minute, "whole audit deadline; local file I/O is not preemptive")
	if err := f.Parse(args); err != nil {
		return err
	}
	if parent == nil || out == nil || f.NArg() != 0 || *directory == "" || *setPath == "" || *timeout <= 0 || (*split != "development" && *split != "reserved" && *split != "all") {
		return errors.New("provide -capture, -set, a valid -split and positive -timeout")
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
	r, err := evaluate(ctx, editions, s, hash, *split)
	if err != nil {
		return err
	}
	var body bytes.Buffer
	enc := json.NewEncoder(&body)
	enc.SetIndent("", "  ")
	if err := enc.Encode(r); err != nil {
		return err
	}
	if body.Len() > 16<<20 {
		return errors.New("encoded report byte limit exceeded")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	n, err := out.Write(body.Bytes())
	if err == nil && n != body.Len() {
		err = io.ErrShortWrite
	}
	return err
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
func searchCase(ctx context.Context, o openedEdition, id, form, raw string, refs []reference) (caseReport, error) {
	prepared, err := textterms.Prepare(ctx, raw, textterms.Limits{MaxInputBytes: 4096, MaxTerms: 64, MaxTermBytes: 4096, MaxOutputBytes: 4096})
	if err != nil {
		return caseReport{}, err
	}
	if len(prepared.Terms) == 0 {
		return caseReport{}, errors.New("query has no technical terms")
	}
	ranked, err := o.index.Search(ctx, termsearch.Query{Edition: o.index.ID(), TermsVersion: prepared.Version, Terms: prepared.Terms}, queryLimits())
	if err != nil {
		return caseReport{}, err
	}
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
		if i < 5 {
			judgment := "unjudged"
			if useful {
				judgment = "known_useful"
			}
			r.Top = append(r.Top, hitReport{path, loc.Body.First, loc.Body.After, h.Score, judgment})
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
func evaluate(ctx context.Context, editions []preparedimport.Edition, s evaluationSet, hash, split string) (report, error) {
	r := report{SetSHA256: hash, CaptureSHA256: s.CaptureSHA256, Processing: s.Processing, RankingVersion: termsearch.BM25Version, Split: split, Documents: len(s.Documents), Summaries: []summary{}, Cases: []caseReport{}, Controls: []caseReport{}}
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
				c, err := searchCase(ctx, o, q.ID, sums[fi].Form, raw, refs)
				if err != nil {
					return report{}, fmt.Errorf("query %q edition %q: %w", q.ID, input.ID, err)
				}
				r.Cases = append(r.Cases, c)
				accumulate(&sums[fi], c)
			}
		}
		r.Summaries = append(r.Summaries, sums[:]...)
		if split != "reserved" {
			for _, q := range s.Controls {
				c, err := searchCase(ctx, o, q.ID, "control", q.Query, nil)
				if err != nil {
					return report{}, err
				}
				r.Controls = append(r.Controls, c)
			}
		}
	}
	if len(r.Cases) == 0 {
		return report{}, errors.New("selected split contains no query intents")
	}
	return r, nil
}
