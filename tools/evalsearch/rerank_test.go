package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/sidsharma96/concordance/internal/htmlarticle"
	"github.com/sidsharma96/concordance/internal/htmlarticlejson"
	"github.com/sidsharma96/concordance/internal/htmledition"
)

func fixtureScores(b rerankBundle) rerankScores {
	wire, _ := auditBytes(b, 128<<20)
	s := rerankScores{Format: "concordance.rerank-scores/v1", BundleSHA256: fmt.Sprintf("%x", sha256.Sum256(wire)), Runtime: rerankRuntime{
		Model: rerankModel, Revision: rerankRevision, WeightsSHA256: rerankWeights, TokenizerSHA256: rerankTokenizer, Device: "cpu", DType: "float32", Attention: "sdpa",
		Python: "3.14.2", Torch: "2.15.0", Transformers: "5.22.1", Tokenizers: "0.23.3", Safetensors: "0.8.0", BatchSize: 1, IntraopThreads: 2, InteropThreads: 1, MaxPairTokens: 8192, InputPolicy: "complete/no-truncation/v1", RunnerSHA256: strings.Repeat("a", 64), DependencyLockSHA256: strings.Repeat("b", 64)}}
	for _, c := range b.Cases {
		for rank, h := range c.Candidates {
			v := float64(rank)
			s.Entries = append(s.Entries, rerankScore{h.PairID, 3, &v})
		}
	}
	return s
}
func writeScores(t *testing.T, path string, s rerankScores) {
	t.Helper()
	body, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
}
func exportFixture(t *testing.T) (string, string, evaluationSet, rerankBundle) {
	t.Helper()
	root, path, s := fixture(t)
	var out bytes.Buffer
	if err := run(t.Context(), []string{"-capture", root, "-set", path, "-rerank-export"}, &out); err != nil {
		t.Fatal(err)
	}
	var b rerankBundle
	if err := json.Unmarshal(out.Bytes(), &b); err != nil {
		t.Fatal(err)
	}
	return root, path, s, b
}
func TestRerankExportScoreRoundTripAndTies(t *testing.T) {
	root, path, s, b := exportFixture(t)
	if b.Model != rerankModel || b.InputVersion != rerankInputVersion || len(b.Cases) != 3 || len(b.Cases[0].Candidates) != 2 || len(b.Cases[2].Candidates) != 0 {
		t.Fatal("wrong bundle")
	}
	raw, _ := json.Marshal(b)
	if bytes.Contains(raw, []byte("known_useful")) || bytes.Contains(raw, []byte("rationale")) || bytes.Contains(raw, []byte("reserved-question")) {
		t.Fatal("labels/reserved query exported")
	}
	h := b.Cases[0].Candidates[0]
	if h.ModelText != "Body:\nPods\nretry safely" || !digest(h.PairID) || h.PairID == b.Cases[0].Candidates[1].PairID {
		t.Fatal("text or identity wrong", h)
	}
	scorePath := filepath.Join(t.TempDir(), "scores.json")
	scores := fixtureScores(b)
	writeScores(t, scorePath, scores)
	var out bytes.Buffer
	args := []string{"-capture", root, "-set", path, "-rerank-scores", scorePath}
	if err := run(t.Context(), args, &out); err != nil {
		t.Fatal(err)
	}
	var r report
	if err := json.Unmarshal(out.Bytes(), &r); err != nil {
		t.Fatal(err)
	}
	if r.Reranked == nil || r.AnyWord == nil || len(r.Reranked.Cases) != 2 || len(r.Reranked.Controls) != 1 {
		t.Fatal("missing comparison")
	}
	for _, c := range r.Reranked.Cases {
		if c.IndexedMatches != 2 || c.Candidates != 2 || c.FirstKnownRankInWindow != 1 || c.Top[0].Path != "docs/b.md" || c.Top[0].LexicalRank != 2 || c.Top[0].Quote != "Pods\nretry safely" {
			t.Fatal("rank/provenance mismatch", c)
		}
	}
	for i := range scores.Entries {
		v := 0.0
		scores.Entries[i].Score = &v
	}
	if err := validateRerankScores(t.Context(), scores, b); err != nil {
		t.Fatal(err)
	}
	first := b.Cases[0].Candidates[0].PairID
	ties, err := describeReranked(t.Context(), b, scores, s)
	if err != nil {
		t.Fatal(err)
	}
	if ties.Cases[0].Top[0].Path != "docs/a.md" || ties.Cases[0].Top[0].Judgment != "unjudged" || ties.Cases[0].FirstKnownRankInWindow != 2 || b.Cases[0].Candidates[0].PairID != first {
		t.Fatal("ties/input mutated")
	}
	if err := run(t.Context(), args, shortWriter{}); !errors.Is(err, io.ErrShortWrite) {
		t.Fatal("short write hidden", err)
	}
}
func TestRerankScoreBindings(t *testing.T) {
	_, _, _, b := exportFixture(t)
	for _, fault := range []string{"bundle", "runtime", "weights", "tokenizer", "revision", "missing", "duplicate", "unknown", "null-score", "nan", "infinite", "tokens", "runner"} {
		t.Run(fault, func(t *testing.T) {
			s := fixtureScores(b)
			switch fault {
			case "bundle":
				s.BundleSHA256 = strings.Repeat("0", 64)
			case "runtime":
				s.Runtime.DType = "float16"
			case "weights":
				s.Runtime.WeightsSHA256 = strings.Repeat("0", 64)
			case "tokenizer":
				s.Runtime.TokenizerSHA256 = strings.Repeat("0", 64)
			case "revision":
				s.Runtime.Revision = strings.Repeat("0", 40)
			case "missing":
				s.Entries = s.Entries[:len(s.Entries)-1]
			case "duplicate":
				s.Entries[1] = s.Entries[0]
			case "unknown":
				s.Entries[0].PairID = strings.Repeat("0", 64)
			case "null-score":
				s.Entries[0].Score = nil
			case "nan":
				v := math.NaN()
				s.Entries[0].Score = &v
			case "infinite":
				v := math.Inf(1)
				s.Entries[0].Score = &v
			case "tokens":
				s.Entries[0].Tokens = 8193
			case "runner":
				s.Runtime.RunnerSHA256 = "missing"
			}
			if err := validateRerankScores(t.Context(), s, b); err == nil {
				t.Fatal("invalid scores accepted", fault)
			}
		})
	}
	for _, field := range []string{"query", "text", "article", "location"} {
		s := fixtureScores(b)
		changed := b
		changed.Cases = slices.Clone(b.Cases)
		changed.Cases[0].Candidates = slices.Clone(b.Cases[0].Candidates)
		switch field {
		case "query":
			changed.Cases[0].Query += "!"
		case "text":
			changed.Cases[0].Candidates[0].ModelText += "!"
		case "article":
			changed.Cases[0].Candidates[0].ArticleSHA256 = strings.Repeat("0", 64)
		case "location":
			changed.Cases[0].Candidates[0].After++
		}
		if err := validateRerankScores(t.Context(), s, changed); err == nil {
			t.Fatal("changed input accepted", field)
		}
	}
	valid, _ := json.Marshal(fixtureScores(b))
	for _, fault := range []string{"unknown-field", "duplicate-root", "duplicate-nested", "case-duplicate", "case-key", "unicode-fold-key", "trailing", "surrogate", "utf8", "null", "missing-score", "null-score", "depth"} {
		wire := slices.Clone(valid)
		switch fault {
		case "unknown-field":
			wire = append([]byte(`{"unexpected":1,`), wire[1:]...)
		case "duplicate-root":
			wire = append([]byte(`{"format":"duplicate",`), wire[1:]...)
		case "duplicate-nested":
			wire = bytes.Replace(wire, []byte(`"device":"cpu"`), []byte(`"device":"mps","device":"cpu"`), 1)
		case "case-duplicate":
			wire = bytes.Replace(wire, []byte(`"score":0`), []byte(`"score":0,"Score":1`), 1)
		case "case-key":
			wire = bytes.Replace(wire, []byte(`"score":0`), []byte(`"SCORE":0`), 1)
		case "unicode-fold-key":
			wire = bytes.Replace(wire, []byte(`"score":0`), []byte(`"ſcore":0`), 1)
		case "trailing":
			wire = append(wire, []byte(" {}")...)
		case "surrogate":
			wire = bytes.Replace(wire, []byte("complete/no-truncation/v1"), []byte(`\ud800`), 1)
		case "utf8":
			wire = append(wire, 0xff)
		case "null":
			wire = []byte("null")
		case "missing-score":
			wire = bytes.Replace(wire, []byte(`,"score":0`), nil, 1)
		case "null-score":
			wire = bytes.Replace(wire, []byte(`"score":0`), []byte(`"score":null`), 1)
		case "depth":
			wire = []byte(strings.Repeat("[", 10) + "0" + strings.Repeat("]", 10))
		}
		path := filepath.Join(t.TempDir(), "bad.json")
		if err := os.WriteFile(path, wire, 0600); err != nil {
			t.Fatal(err)
		}
		if got, err := loadRerankScores(t.Context(), path, b); err == nil || len(got.Entries) != 0 {
			t.Fatal("malformed file accepted", fault, err)
		}
	}
}
func TestRerankAdmissionAndReservedPreflight(t *testing.T) {
	root, path, s, _ := exportFixture(t)
	base := []string{"-capture", root, "-set", path}
	for _, extra := range [][]string{{"-rerank-export", "-split", "reserved"}, {"-rerank-scores", "absent", "-split", "all"}, {"-rerank-export", "-compare-any"}, {"-rerank-scores", "absent", "-compare-noise"}, {"-rerank-export", "-rerank-scores", "absent"}} {
		var out bytes.Buffer
		if err := run(t.Context(), append(slices.Clone(base), extra...), &out); err == nil || out.Len() != 0 {
			t.Fatal("bad admission", extra, err)
		}
	}
	for _, extra := range [][]string{{"-rerank-export"}, {"-rerank-scores", "absent"}} {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		var out bytes.Buffer
		if err := run(ctx, append(slices.Clone(base), extra...), &out); !errors.Is(err, context.Canceled) || out.Len() != 0 {
			t.Fatal("cancellation", err)
		}
	}
	s.Intents[1].Answers[0].References[0].QuoteSHA256 = strings.Repeat("0", 64)
	writeSet(t, path, s)
	var out bytes.Buffer
	if err := run(t.Context(), append(base, "-rerank-export"), &out); err == nil || !strings.Contains(err.Error(), "reference quote mismatch") || out.Len() != 0 {
		t.Fatal("reserved bindings skipped", err)
	}
}
func TestRerankWindowAndModelText(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	html := `<main class="fixture"><h1>Pods</h1>` + strings.Repeat("<p>retry safely</p>", 125) + "</main>"
	a, err := htmlarticle.Extract(ctx, []byte(html), htmlarticle.Profile{ID: "fixture/v1", ArticleClass: "fixture"}, extractLimits())
	if err != nil {
		t.Fatal(err)
	}
	body, err := htmlarticlejson.Encode(ctx, a, buildLimits().Article)
	if err != nil {
		t.Fatal(err)
	}
	index, err := htmledition.Build(ctx, "one", [][]byte{body}, buildLimits())
	if err != nil {
		t.Fatal(err)
	}
	o := openedEdition{index: index, bodies: [][]byte{body}, docs: []document{{Path: "docs/window.md", ArticleSHA256: hexHash(body)}}, positions: map[location]int{}}
	article, err := index.Article(0)
	if err != nil {
		t.Fatal(err)
	}
	for pos := range article.Passages {
		loc, err := index.Location(pos)
		if err != nil {
			t.Fatal(err)
		}
		o.positions[location{"docs/window.md", loc.Body.First, loc.Body.After}] = pos
	}
	c, err := exportRerankCase(ctx, o, "window", "keywords", "retry")
	if err != nil {
		t.Fatal(err)
	}
	if c.IndexedMatches != 125 || len(c.Candidates) != 100 {
		t.Fatal("wrong window/total", c.IndexedMatches, len(c.Candidates))
	}
	for i, h := range c.Candidates {
		if h.Quote == "" || h.ArticleSHA256 != hexHash(body) || !digest(h.PairID) {
			t.Fatal("lost evidence", i)
		}
		if i > 0 && h.LexicalScore > c.Candidates[i-1].LexicalScore {
			t.Fatal("not lexical prefix")
		}
	}
	h := modelText(hitReport{Headings: []string{"Pods"}, Labels: []string{"Note:"}, Quote: "Full\nbody"})
	if h != "Heading: Pods\nNotice: Note:\nBody:\nFull\nbody" {
		t.Fatal("model text changed", h)
	}
}
