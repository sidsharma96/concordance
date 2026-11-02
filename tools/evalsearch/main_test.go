package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sidsharma96/concordance/internal/capturefiles"
	"github.com/sidsharma96/concordance/internal/htmlarticle"
	"github.com/sidsharma96/concordance/internal/htmlarticlejson"
	"github.com/sidsharma96/concordance/internal/htmlpassage"
	"github.com/sidsharma96/concordance/internal/htmlterms"
	"github.com/sidsharma96/concordance/internal/textterms"
)

func hexHash(b []byte) string { return fmt.Sprintf("%x", sha256.Sum256(b)) }
func fixture(t *testing.T) (string, string, evaluationSet) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	root := t.TempDir()
	html := []byte(`<main class="fixture"><h1>Pods</h1><p>retry safely</p><p>unrelated words</p></main>`)
	recipe := []byte("authored fixture")
	profile := htmlarticle.Profile{ID: "fixture/v1", ArticleClass: "fixture"}
	ref := func(path string, b []byte) capturefiles.File {
		return capturefiles.File{Path: path, SHA256: sha256.Sum256(b), Bytes: int64(len(b))}
	}
	desc := capturefiles.Description{Editions: []capturefiles.Edition{{ID: "one", Captures: []capturefiles.Capture{{Source: "docs", Revision: strings.Repeat("a", 40), Directory: "docs", Preparation: "fixture/v1", Toolchain: "authored", Profile: profile, Recipe: ref("recipe", recipe), Pages: []capturefiles.Page{{Path: "docs/a.md", HTML: ref("page", html)}, {Path: "docs/b.md", HTML: ref("page", html)}}}}}}}
	descriptor, err := capturefiles.Encode(ctx, desc, captureLimits())
	if err != nil {
		t.Fatal(err)
	}
	for name, b := range map[string][]byte{"recipe": recipe, "page": html, capturefiles.DescriptorName: descriptor} {
		if err := os.WriteFile(filepath.Join(root, name), b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	a, err := htmlarticle.Extract(ctx, html, profile, extractLimits())
	if err != nil {
		t.Fatal(err)
	}
	body, err := htmlarticlejson.Encode(ctx, a, buildLimits().Article)
	if err != nil {
		t.Fatal(err)
	}
	doc := func(path string) document {
		return document{Edition: "one", Source: "docs", Revision: strings.Repeat("a", 40), Path: path, ArticleSHA256: hexHash(body)}
	}
	q := intent{ID: "development-question", Topic: "fixture", Split: "development", Need: "Find safe retry instructions", Keywords: "retry", Natural: "retry safely", Rationale: "Authored paragraph supplies the requested action", Answers: []answer{{Edition: "one", References: []reference{{Path: "docs/b.md", First: 0, After: 2, QuoteSHA256: hexHash([]byte("Pods\nretry safely"))}}}}}
	reserved := q
	reserved.ID = "reserved-question"
	reserved.Split = "reserved"
	reserved.Answers = []answer{{Edition: "one", References: append([]reference(nil), q.Answers[0].References...)}}
	s := evaluationSet{Format: "concordance.retrieval-eval/v1", Status: "reviewed", Pins: map[string]string{"one": strings.Repeat("a", 40)}, CaptureSHA256: hexHash(descriptor), Processing: processing{Article: htmlarticle.Processor, Profile: profile.ID, Passages: htmlpassage.Processor, Fields: htmlterms.Processor, Terms: textterms.Version}, Documents: []document{doc("docs/a.md"), doc("docs/b.md")}, Intents: []intent{q, reserved}, Controls: []control{{ID: "outside", Query: "unavailablefeaturexyz", Rationale: "Absent authored identifier"}}}
	name := filepath.Join(t.TempDir(), "set.json")
	writeSet(t, name, s)
	return root, name, s
}
func writeSet(t *testing.T, path string, s evaluationSet) {
	t.Helper()
	b, e := json.Marshal(s)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(path, b, 0600); e != nil {
		t.Fatal(e)
	}
}
func TestAuditUsesKnownAddressesNotSharedBytes(t *testing.T) {
	root, path, _ := fixture(t)
	for _, split := range []string{"development", "reserved", "all"} {
		t.Run(split, func(t *testing.T) {
			var out bytes.Buffer
			if err := run(t.Context(), []string{"-capture", root, "-set", path, "-split", split}, &out); err != nil {
				t.Fatal(err)
			}
			var r report
			if err := json.Unmarshal(out.Bytes(), &r); err != nil {
				t.Fatal(err)
			}
			cases, controls := 2, 1
			if split == "reserved" {
				controls = 0
			}
			if split == "all" {
				cases = 4
			}
			if len(r.Cases) != cases || len(r.Controls) != controls || r.References != 2 {
				t.Fatal(r)
			}
			for _, c := range r.Cases {
				if c.Matches != 2 || c.FirstKnownRank != 2 || len(c.Top) != 2 || c.Top[0].Judgment != "unjudged" || c.Top[1].Judgment != "known_useful" {
					t.Fatalf("unknown is not irrelevant or the same logical document: %+v", c)
				}
			}
			for _, s := range r.Summaries {
				if s.KnownAtOne != 0 || s.KnownAtFive != s.Cases || s.ZeroMatches != 0 {
					t.Fatal(s)
				}
			}
		})
	}
}
func TestFixtureAndQueryErrorsHaveNoPartialReport(t *testing.T) {
	for _, name := range []string{"article", "quote", "range", "reserved-quote", "inventory", "unknown-document-edition", "missing-rationale", "processing", "query", "duplicate-id", "capture", "unreviewed"} {
		t.Run(name, func(t *testing.T) {
			root, path, s := fixture(t)
			switch name {
			case "article":
				s.Documents[0].ArticleSHA256 = strings.Repeat("0", 64)
			case "quote":
				s.Intents[0].Answers[0].References[0].QuoteSHA256 = strings.Repeat("0", 64)
			case "range":
				s.Intents[0].Answers[0].References[0].First = 1
			case "reserved-quote":
				s.Intents[1].Answers[0].References[0].QuoteSHA256 = strings.Repeat("0", 64)
				if s.Intents[0].Answers[0].References[0].QuoteSHA256 == strings.Repeat("0", 64) {
					t.Fatal("fixture references alias across intents")
				}
			case "inventory":
				s.Documents = s.Documents[:1]
			case "unknown-document-edition":
				d := s.Documents[0]
				d.Edition, d.Revision = "unknown", ""
				s.Documents = append(s.Documents, d)
			case "missing-rationale":
				s.Intents[0].Rationale = ""
			case "processing":
				s.Processing.Terms = "incompatible"
			case "query":
				s.Intents[0].Natural = "!!!"
			case "duplicate-id":
				s.Controls[0].ID = s.Intents[0].ID
			case "capture":
				s.CaptureSHA256 = strings.Repeat("0", 64)
			case "unreviewed":
				s.Status = "proposed-labels"
			}
			writeSet(t, path, s)
			var out bytes.Buffer
			if err := run(t.Context(), []string{"-capture", root, "-set", path}, &out); err == nil || out.Len() != 0 {
				t.Fatal("expected preflight/query failure without a body", err, out.String())
			}
		})
	}
}

type shortWriter struct{}

func (shortWriter) Write(b []byte) (int, error) { return len(b) - 1, nil }
func TestBoundariesAndCheckedOutput(t *testing.T) {
	root, path, _ := fixture(t)
	for _, args := range [][]string{{}, {"-capture", root, "-set", path, "extra"}, {"-capture", root, "-set", path, "-split", "bad"}, {"-capture", root, "-set", path, "-timeout", "0s"}} {
		var out bytes.Buffer
		if err := run(t.Context(), args, &out); err == nil || out.Len() != 0 {
			t.Fatal(args, err)
		}
	}
	if err := run(t.Context(), []string{"-capture", root, "-set", path}, shortWriter{}); !errors.Is(err, io.ErrShortWrite) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	var out bytes.Buffer
	if err := run(ctx, []string{"-capture", root, "-set", path}, &out); !errors.Is(err, context.Canceled) || out.Len() != 0 {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, bytes.Repeat([]byte(" "), (8<<20)+1), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := loadSet(path); err == nil {
		t.Fatal("oversized set admitted")
	}
}
func TestKnownRankFiveBoundary(t *testing.T) {
	var s summary
	for _, c := range []caseReport{{Matches: 0}, {Matches: 8, FirstKnownRank: 1}, {Matches: 8, FirstKnownRank: 5}, {Matches: 8, FirstKnownRank: 6}} {
		accumulate(&s, c)
	}
	if s.Cases != 4 || s.KnownAtOne != 1 || s.KnownAtFive != 2 || s.ZeroMatches != 1 {
		t.Fatal(s)
	}
}
