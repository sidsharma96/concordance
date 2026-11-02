package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"
)

type processing struct {
	Article  string `json:"article"`
	Profile  string `json:"profile"`
	Passages string `json:"passages"`
	Fields   string `json:"fields"`
	Terms    string `json:"terms"`
}
type document struct {
	Edition       string `json:"edition"`
	Source        string `json:"source"`
	Revision      string `json:"revision"`
	Path          string `json:"path"`
	ArticleSHA256 string `json:"article_sha256"`
}
type reference struct {
	Path        string `json:"path"`
	First       int    `json:"first"`
	After       int    `json:"after"`
	QuoteSHA256 string `json:"quote_sha256"`
}
type answer struct {
	Edition    string      `json:"edition"`
	References []reference `json:"references"`
}
type intent struct {
	ID        string   `json:"id"`
	Topic     string   `json:"topic"`
	Split     string   `json:"split"`
	Need      string   `json:"need"`
	Keywords  string   `json:"keywords"`
	Natural   string   `json:"natural"`
	Rationale string   `json:"rationale"`
	Answers   []answer `json:"answers"`
}
type control struct {
	ID        string `json:"id"`
	Query     string `json:"query"`
	Rationale string `json:"rationale"`
}
type evaluationSet struct {
	Format        string            `json:"format"`
	Status        string            `json:"status"`
	Pins          map[string]string `json:"pins"`
	CaptureSHA256 string            `json:"capture_sha256"`
	Processing    processing        `json:"processing"`
	Documents     []document        `json:"documents"`
	Intents       []intent          `json:"intents"`
	Controls      []control         `json:"controls"`
}

// The input is operator-trusted fixture configuration, not an authenticated
// artifact codec. The encoded-byte cap also bounds decoding before count checks.
func readBounded(name string, limit int64) ([]byte, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > limit {
		return nil, errors.Join(errors.New("expected bounded regular file"), err, f.Close())
	}
	b, re := io.ReadAll(io.LimitReader(f, limit+1))
	if err := errors.Join(re, f.Close()); err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, errors.New("file byte limit exceeded")
	}
	return b, nil
}
func loadSet(name string) (evaluationSet, string, error) {
	body, err := readBounded(name, 8<<20)
	if err != nil {
		return evaluationSet{}, "", err
	}
	if !utf8.Valid(body) {
		return evaluationSet{}, "", errors.New("set is not UTF-8")
	}
	var s evaluationSet
	d := json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	if err := d.Decode(&s); err != nil {
		return evaluationSet{}, "", err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return evaluationSet{}, "", errors.New("trailing set data")
	}
	if err := s.validate(); err != nil {
		return evaluationSet{}, "", err
	}
	return s, fmt.Sprintf("%x", sha256.Sum256(body)), nil
}
func text(s string) bool {
	return s != "" && len(s) <= 4096 && utf8.ValidString(s) && !strings.ContainsRune(s, 0)
}
func digest(s string) bool {
	b, e := hex.DecodeString(s)
	return e == nil && len(b) == 32 && hex.EncodeToString(b) == s
}
func (s evaluationSet) validate() error {
	if s.Format != "concordance.retrieval-eval/v1" || s.Status != "reviewed" || !digest(s.CaptureSHA256) || len(s.Pins) == 0 || len(s.Pins) > 8 || len(s.Documents) == 0 || len(s.Documents) > 100 || len(s.Intents) == 0 || len(s.Intents) > 64 || len(s.Controls) > 32 {
		return errors.New("invalid or unreviewed evaluation set")
	}
	for ed, rev := range s.Pins {
		if !text(ed) || !text(rev) {
			return errors.New("invalid source pin")
		}
	}
	docs := map[string]bool{}
	for _, d := range s.Documents {
		k := d.Edition + "\x00" + d.Path
		revision, pinned := s.Pins[d.Edition]
		if !pinned || !text(d.Source) || !text(d.Path) || !text(d.Edition) || revision != d.Revision || !digest(d.ArticleSHA256) || docs[k] {
			return errors.New("invalid/duplicate document binding")
		}
		docs[k] = true
	}
	ids := map[string]bool{}
	refs := 0
	for _, q := range s.Intents {
		if !text(q.ID) || ids[q.ID] || !text(q.Topic) || !text(q.Need) || !text(q.Keywords) || !text(q.Natural) || !text(q.Rationale) || (q.Split != "development" && q.Split != "reserved") || len(q.Answers) != len(s.Pins) {
			return errors.New("invalid/duplicate intent")
		}
		ids[q.ID] = true
		seen := map[string]bool{}
		for _, a := range q.Answers {
			if _, ok := s.Pins[a.Edition]; !ok || seen[a.Edition] || len(a.References) == 0 || len(a.References) > 100 {
				return errors.New("invalid answer editions/references")
			}
			seen[a.Edition] = true
			seenRef := map[reference]bool{}
			for _, r := range a.References {
				if !docs[a.Edition+"\x00"+r.Path] || r.First < 0 || r.After <= r.First || !digest(r.QuoteSHA256) || seenRef[r] {
					return errors.New("invalid/duplicate answer reference")
				}
				seenRef[r] = true
				refs++
			}
		}
	}
	if refs > 10000 {
		return errors.New("too many reference occurrences")
	}
	for _, c := range s.Controls {
		if !text(c.ID) || ids[c.ID] || !text(c.Query) || !text(c.Rationale) {
			return errors.New("invalid/duplicate control")
		}
		ids[c.ID] = true
	}
	return nil
}
