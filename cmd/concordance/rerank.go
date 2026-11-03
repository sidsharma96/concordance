package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/sidsharma96/concordance/internal/htmlpassage"
	"github.com/sidsharma96/concordance/internal/publishedsearch"
	"github.com/sidsharma96/concordance/internal/rerank"
	"github.com/sidsharma96/concordance/internal/termsearch"
)

func collectReranked(ctx context.Context, r *publishedsearch.Reader, raw string, q termsearch.Query, search termsearch.Limits, read readOptions, socket string, quoteBytes, totalBytes, limit int, articleBytes int64) (publishedsearch.QuotedResult, int, error) {
	candidates, err := r.SearchAnyEvidenceGroupedFirst(ctx, q, search, publishedsearch.GroupedCandidateLimits{
		Evidence: publishedsearch.EvidenceLimits{Passages: htmlpassage.Limits{MaxBlocks: 200000, MaxGroups: 200000, MaxInputBytes: read.article, MaxPassages: read.passages, MaxBodyBytes: read.article, MaxReferences: 2000000, MaxContextBytes: 64 << 20}, MaxQuoteBytes: quoteBytes, MaxContextBytes: 64 << 10}, MaxDistinctArticleBytes: articleBytes, MaxTextBytes: 8 << 20,
	}, rerank.MaxPairs)
	if err != nil {
		return publishedsearch.QuotedResult{}, 0, err
	}
	inputs := make([]rerank.Input, len(candidates.Candidates))
	for i, c := range candidates.Candidates {
		inputs[i] = rerank.Input{Body: c.Evidence.Quote, Headings: c.Evidence.Headings, Labels: c.Evidence.Labels}
	}
	batch, err := rerank.Prepare(ctx, raw, inputs)
	if err != nil {
		return publishedsearch.QuotedResult{}, 0, err
	}
	if batch.Len() > 0 {
		socket, err = workerSocket(socket)
		if err != nil {
			return publishedsearch.QuotedResult{}, 0, err
		}
	}
	scores, err := rerank.Score(ctx, socket, batch)
	if err != nil {
		return publishedsearch.QuotedResult{}, 0, err
	}
	order := make([]int, len(inputs))
	for i := range order {
		order[i] = i
	}
	// Stable ties keep the original lexical order; model scores are raw logits.
	slices.SortStableFunc(order, func(a, b int) int {
		if scores.Scores[a] > scores.Scores[b] {
			return -1
		}
		if scores.Scores[a] < scores.Scores[b] {
			return 1
		}
		return 0
	})
	out := publishedsearch.QuotedResult{RankingVersion: "bge-v2-m3/lexical100/v1", TotalMatches: candidates.TotalMatches, Hits: make([]publishedsearch.QuotedHit, min(limit, len(order)))}
	remaining := totalBytes
	for i, position := range order[:len(out.Hits)] {
		if err := ctx.Err(); err != nil {
			return publishedsearch.QuotedResult{}, 0, err
		}
		c := candidates.Candidates[position]
		// The candidate budget covers all model inputs; this budget covers only
		// the final requested display prefix after every candidate was scored.
		if len(c.Evidence.Quote) > remaining {
			return publishedsearch.QuotedResult{}, 0, publishedsearch.ErrLimit
		}
		remaining -= len(c.Evidence.Quote)
		out.Hits[i] = publishedsearch.QuotedHit{Ref: c.Ref, Score: scores.Scores[position], Quote: c.Evidence.Quote}
	}
	return out, len(order), nil
}
func workerSocket(value string) (string, error) {
	if value == "" || !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
		return "", errors.New("configure --worker-socket or CONCORDANCE_WORKER_SOCKET for reranking")
	}
	path, err := filepath.Abs(value)
	if err != nil || len(path) > 103 {
		return "", errors.New("provide a short local worker socket path")
	}
	return path, nil
}
func renderReranked(ctx context.Context, r *publishedsearch.Reader, name string, selection publishedsearch.Selection, batch publishedsearch.QuotedResult, candidates, maxBytes int) ([]byte, error) {
	b := displayBuffer{max: maxBytes}
	if _, err := fmt.Fprintf(&b, "Local model reranking | scored %d keyword candidates | model %s\n", candidates, rerank.Model); err != nil {
		return nil, err
	}
	body, err := renderSearch(ctx, r, name, selection, batch, maxBytes-len(b.body))
	if err != nil {
		return nil, err
	}
	if _, err = b.Write(body); err != nil {
		return nil, err
	}
	return b.body, nil
}
