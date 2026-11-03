package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestExplicitReservedRerankValidation(t *testing.T) {
	root, path, s := fixture(t)
	s.Intents[0].Natural = "!!!" // A development query would fail if accidentally executed.
	writeSet(t, path, s)
	base := []string{"-capture", root, "-set", path, "-split", "reserved", "-validate-reserved"}
	var out bytes.Buffer
	if err := run(t.Context(), append(slices.Clone(base), "-rerank-export"), &out); err != nil {
		t.Fatal(err)
	}
	var b rerankBundle
	if err := json.Unmarshal(out.Bytes(), &b); err != nil {
		t.Fatal(err)
	}
	if b.ValidationSplit != "reserved" || len(b.Cases) != 2 || b.Cases[0].ID != "reserved-question" || b.Cases[1].ID != "reserved-question" || b.Cases[0].Form != "keywords" || b.Cases[1].Form != "natural" {
		t.Fatal("wrong reserved scope", b.Cases)
	}
	// No labels, development query or control is in the model bundle.
	if bytes.Contains(out.Bytes(), []byte("development-question")) || bytes.Contains(out.Bytes(), []byte("known_useful")) || bytes.Contains(out.Bytes(), []byte("outside")) {
		t.Fatal("wrong export contents")
	}
	scorePath := filepath.Join(t.TempDir(), "scores.json")
	scores := fixtureScores(b)
	writeScores(t, scorePath, scores)
	out.Reset()
	if err := run(t.Context(), append(slices.Clone(base), "-rerank-scores", scorePath), &out); err != nil {
		t.Fatal(err)
	}
	var r report
	if err := json.Unmarshal(out.Bytes(), &r); err != nil {
		t.Fatal(err)
	}
	if r.Split != "reserved" || r.Reranked == nil || r.Reranked.ValidationSplit != "reserved" || len(r.Reranked.Cases) != 2 || len(r.Reranked.Controls) != 0 || len(r.Controls) != 0 || len(r.AnyWord.Controls) != 0 || r.References != 2 {
		t.Fatal("mixed reserved/development/controls", r)
	}
	for _, c := range r.Reranked.Cases {
		if c.ID != "reserved-question" || c.FirstKnownRankInWindow != 1 {
			t.Fatal("reserved labels/rank mismatch", c)
		}
	}
	// Scope participates in bundle fingerprint even when the selected cases are identical.
	changed := b
	changed.ValidationSplit = ""
	if err := validateRerankScores(t.Context(), scores, changed); err == nil {
		t.Fatal("reserved marker not bound")
	}
	// Full preflight still verifies development references without querying them.
	s.Intents[0].Answers[0].References[0].QuoteSHA256 = strings.Repeat("0", 64)
	writeSet(t, path, s)
	out.Reset()
	if err := run(t.Context(), append(base, "-rerank-export"), &out); err == nil || !strings.Contains(err.Error(), "reference quote mismatch") || out.Len() != 0 {
		t.Fatal("development preflight skipped", err)
	}
}
func TestReservedValidationAdmissionBeforeFileAccess(t *testing.T) {
	base := []string{"-capture", "absent", "-set", "absent"}
	for _, args := range [][]string{
		{"-validate-reserved"},
		{"-validate-reserved", "-rerank-export"},
		{"-validate-reserved", "-split", "all", "-rerank-export"},
		{"-validate-reserved", "-split", "reserved"},
		{"-validate-reserved", "-split", "reserved", "-compare-any", "-rerank-export"},
		{"-validate-reserved", "-split", "reserved", "-compare-noise", "-rerank-scores", "absent"},
		{"-validate-reserved", "-split", "reserved", "-rerank-export", "-rerank-scores", "absent"},
		{"-split", "reserved", "-rerank-export"},
		{"-split", "reserved", "-rerank-scores", "absent"},
		{"-split", "all", "-rerank-export"},
	} {
		var out bytes.Buffer
		err := run(t.Context(), append(slices.Clone(base), args...), &out)
		if err == nil || out.Len() != 0 || strings.Contains(err.Error(), "load set") {
			t.Fatal("validation guard did file work", args, err)
		}
	}
}
func TestDevelopmentRerankWireRemainsUnmarked(t *testing.T) {
	_, _, _, b := exportFixture(t)
	wire, err := auditBytes(b, 128<<20)
	if err != nil {
		t.Fatal(err)
	}
	if b.ValidationSplit != "" || bytes.Contains(wire, []byte("validation_split")) {
		t.Fatal("development wire changed")
	}
	scorePath := filepath.Join(t.TempDir(), "scores.json")
	writeScores(t, scorePath, fixtureScores(b))
	s, err := loadRerankScores(t.Context(), scorePath, b)
	if err != nil {
		t.Fatal(err)
	}
	_, _, set, _ := exportFixture(t)
	r, err := describeReranked(t.Context(), b, s, set)
	if err != nil {
		t.Fatal(err)
	}
	wire, err = json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(wire, []byte("validation_split")) {
		t.Fatal("development report gained field")
	}
	if _, err := os.Stat(scorePath); err != nil {
		t.Fatal(err)
	}
}
