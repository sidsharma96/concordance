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
	"unicode/utf8"

	"github.com/sidsharma96/concordance/internal/jsonunicode"
)

type rerankRuntime struct {
	RunnerSHA256         string `json:"runner_sha256"`
	DependencyLockSHA256 string `json:"dependency_lock_sha256"`
	Model                string `json:"model"`
	Revision             string `json:"revision"`
	WeightsSHA256        string `json:"weights_sha256"`
	TokenizerSHA256      string `json:"tokenizer_sha256"`
	Device               string `json:"device"`
	DType                string `json:"dtype"`
	Attention            string `json:"attention"`
	Python               string `json:"python"`
	Torch                string `json:"torch"`
	Transformers         string `json:"transformers"`
	Tokenizers           string `json:"tokenizers"`
	Safetensors          string `json:"safetensors"`
	BatchSize            int    `json:"batch_size"`
	IntraopThreads       int    `json:"intraop_threads"`
	InteropThreads       int    `json:"interop_threads"`
	MaxPairTokens        int    `json:"max_pair_tokens"`
	InputPolicy          string `json:"input_policy"`
}
type rerankScore struct {
	PairID string   `json:"pair_id"`
	Tokens int      `json:"tokens"`
	Score  *float64 `json:"score"`
}
type rerankScores struct {
	Format       string        `json:"format"`
	BundleSHA256 string        `json:"bundle_sha256"`
	Runtime      rerankRuntime `json:"runtime"`
	Entries      []rerankScore `json:"entries"`
}

func loadRerankScores(ctx context.Context, path string, b rerankBundle) (rerankScores, error) {
	fail := func(err error) (rerankScores, error) { return rerankScores{}, err }
	wire, err := readBounded(path, 8<<20)
	if err != nil {
		return fail(err)
	}
	if !utf8.Valid(wire) {
		return fail(errors.New("score file is not UTF-8"))
	}
	if err := jsonunicode.CheckSurrogates(ctx, wire); err != nil {
		return fail(err)
	}
	tokens := json.NewDecoder(bytes.NewReader(wire))
	if err := uniqueJSON(ctx, tokens, 0); err != nil {
		return fail(err)
	}
	if _, err := tokens.Token(); err != io.EOF {
		return fail(errors.New("trailing score data"))
	}
	var s rerankScores
	d := json.NewDecoder(bytes.NewReader(wire))
	d.DisallowUnknownFields()
	if err := d.Decode(&s); err != nil {
		return fail(err)
	}
	if err := validateRerankScores(ctx, s, b); err != nil {
		return fail(err)
	}
	return s, nil
}
func validateRerankScores(ctx context.Context, s rerankScores, b rerankBundle) error {
	body, err := auditBytes(b, 128<<20)
	if err != nil {
		return err
	}
	if s.Format != "concordance.rerank-scores/v1" || s.BundleSHA256 != fmt.Sprintf("%x", sha256.Sum256(body)) {
		return errors.New("score bundle mismatch")
	}
	r := s.Runtime
	if !digest(r.RunnerSHA256) || !digest(r.DependencyLockSHA256) || r.Model != rerankModel || r.Revision != rerankRevision || r.WeightsSHA256 != rerankWeights || r.TokenizerSHA256 != rerankTokenizer ||
		(r.Device != "cpu" && r.Device != "mps") || r.DType != "float32" || r.Attention != "sdpa" || r.Python != "3.14.2" || r.Torch != "2.15.0" ||
		r.Transformers != "5.22.1" || r.Tokenizers != "0.23.3" || r.Safetensors != "0.8.0" || r.BatchSize != 1 || r.IntraopThreads != 2 ||
		r.InteropThreads != 1 || r.MaxPairTokens != 8192 || r.InputPolicy != "complete/no-truncation/v1" {
		return errors.New("score runtime mismatch")
	}
	expected := map[string]bool{}
	count := 0
	for _, c := range b.Cases {
		for _, h := range c.Candidates {
			if expected[h.PairID] {
				return errors.New("duplicate exported pair")
			}
			expected[h.PairID] = true
			count++
		}
	}
	if len(s.Entries) != count {
		return errors.New("incomplete score coverage")
	}
	for _, row := range s.Entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !expected[row.PairID] || row.Score == nil || math.IsNaN(*row.Score) || math.IsInf(*row.Score, 0) || row.Tokens < 1 || row.Tokens > 8192 {
			return errors.New("invalid, unknown or duplicate score")
		}
		delete(expected, row.PairID)
	}
	if len(expected) != 0 {
		return errors.New("missing pair scores")
	}
	return ctx.Err()
}

// Reject duplicate object fields at every level before struct decoding can choose
// a later value. The 8MiB file cap bounds tokens/maps; nesting is separately capped.
// Grammar and field types are still encoding/json's work.
func uniqueJSON(ctx context.Context, d *json.Decoder, depth int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if depth > 8 {
		return errors.New("score JSON nesting limit")
	}
	t, err := d.Token()
	if err != nil {
		return err
	}
	delim, ok := t.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return err
			}
			s, ok := key.(string)
			if !ok || seen[s] {
				return errors.New("duplicate score JSON field")
			}
			for _, c := range s {
				if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '_' {
					return errors.New("noncanonical score JSON field")
				}
			}
			seen[s] = true
			if err := uniqueJSON(ctx, d, depth+1); err != nil {
				return err
			}
		}
	case '[':
		for d.More() {
			if err := uniqueJSON(ctx, d, depth+1); err != nil {
				return err
			}
		}
	default:
		return errors.New("invalid score JSON delimiter")
	}
	_, err = d.Token()
	return err
}
