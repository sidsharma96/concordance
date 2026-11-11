package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/sidsharma96/concordance/internal/htmlpassage"
	"github.com/sidsharma96/concordance/internal/publishedsearch"
	"github.com/sidsharma96/concordance/internal/rerank"
	"github.com/sidsharma96/concordance/internal/termsearch"
)

// Evidence follows Quotes.Hits in the same selected order. Reranking retains the
// already-verified candidate context instead of fetching displayed articles again.
// Owned strings/slices stay unchanged through rendering; no state is persisted.
type searchBatch struct {
	Quotes           publishedsearch.QuotedResult
	Evidence         []publishedsearch.Evidence
	RankedCandidates int
}

func evidenceLimits(read readOptions, quoteBytes int) publishedsearch.EvidenceLimits {
	return publishedsearch.EvidenceLimits{
		Passages:      htmlpassage.Limits{MaxBlocks: 200000, MaxGroups: 200000, MaxInputBytes: read.article, MaxPassages: read.passages, MaxBodyBytes: read.article, MaxReferences: 2000000, MaxContextBytes: 64 << 20},
		MaxQuoteBytes: quoteBytes, MaxContextBytes: 64 << 10,
	}
}

// Complete ranking precedes selection. Each distinct selected article gets one
// strict read; every passage retains its independent checks and context rebuild.
// O(K+U+B) state for K hits, U articles and B text bytes coexists with full ranking
// and one article/plan's scratch. No article bytes survive this call.
// Body and aggregate body/context admission are distinct; neither is a heap cap.
// Every error returns a zero batch, including after successful earlier reads.
func collectJSONSearch(ctx context.Context, r *publishedsearch.Reader, q termsearch.Query, search termsearch.Limits, read readOptions, quoteBytes, totalBytes, outputBytes, limit int, all bool) (searchBatch, error) {
	if all {
		limit = search.MaxMatches
	}
	ranked, err := r.SearchEvidenceGroupedFirst(ctx, q, search, publishedsearch.GroupedCandidateLimits{
		Evidence: evidenceLimits(read, quoteBytes), MaxDistinctArticleBytes: int64(read.editionArticles), MaxTextBytes: outputBytes,
	}, limit)
	if errors.Is(err, publishedsearch.ErrTextLimit) {
		return searchBatch{}, errOutputLimit
	}
	if err != nil {
		return searchBatch{}, err
	}
	out := searchBatch{Quotes: publishedsearch.QuotedResult{RankingVersion: ranked.RankingVersion, TotalMatches: ranked.TotalMatches, Hits: make([]publishedsearch.QuotedHit, 0, len(ranked.Candidates))}, Evidence: make([]publishedsearch.Evidence, 0, len(ranked.Candidates)), RankedCandidates: ranked.TotalMatches}
	quoteLeft, textLeft := totalBytes, outputBytes
	for _, hit := range ranked.Candidates {
		e := hit.Evidence
		if len(e.Quote) > quoteLeft {
			return searchBatch{}, publishedsearch.ErrLimit
		}
		quoteLeft -= len(e.Quote)
		textLeft, err = admitJSONText(e, textLeft)
		if err != nil {
			return searchBatch{}, err
		}
		out.Quotes.Hits = append(out.Quotes.Hits, publishedsearch.QuotedHit{Ref: hit.Ref, Score: hit.Score, Quote: e.Quote})
		out.Evidence = append(out.Evidence, e)
	}
	if err := ctx.Err(); err != nil {
		return searchBatch{}, err
	}
	return out, nil
}

// UTF-8 text bytes are a lower bound on their JSON encoding. Metadata, escaping
// and separators are charged by the complete encoded-output writer afterward.
func admitJSONText(e publishedsearch.Evidence, remaining int) (int, error) {
	if len(e.Quote) > remaining {
		return 0, errOutputLimit
	}
	remaining -= len(e.Quote)
	for _, texts := range [][]string{e.Headings, e.Labels} {
		for _, text := range texts {
			if len(text) > remaining {
				return 0, errOutputLimit
			}
			remaining -= len(text)
		}
	}
	return remaining, nil
}

const searchJSONFormat = "concordance.docs-search/v1"

// Explicit output types prevent exporting private handles or new internal fields
// by accident. This Git/HTML v1 contract is not a universal source representation.
type searchJSON struct {
	Format    string              `json:"format"`
	Status    string              `json:"status"`
	Target    string              `json:"target"`
	Selection searchJSONSelection `json:"selection"`
	Retrieval searchJSONRetrieval `json:"retrieval"`
	Results   []searchJSONPassage `json:"results"`
}
type searchJSONSelection struct {
	Collection string `json:"collection"`
	Generation string `json:"generation"`
	Edition    string `json:"edition"`
}
type searchJSONRetrieval struct {
	Policy           string           `json:"policy"`
	TermsVersion     string           `json:"terms_version"`
	RankingVersion   string           `json:"ranking_version"`
	IndexedMatches   int              `json:"indexed_matches"`
	RankedCandidates int              `json:"ranked_candidates"`
	Model            *searchJSONModel `json:"model,omitempty"`
}
type searchJSONModel struct {
	ID           string `json:"id"`
	Revision     string `json:"revision"`
	InputVersion string `json:"input_version"`
	Runtime      string `json:"runtime"`
}
type searchJSONPassage struct {
	SourceRef string             `json:"source_ref"`
	ID        string             `json:"id"`
	Source    searchJSONSource   `json:"source"`
	Evidence  searchJSONEvidence `json:"evidence"`
}
type searchJSONSource struct {
	Kind       string `json:"kind"`
	ID         string `json:"id"`
	Repository string `json:"repository"`
	Revision   string `json:"revision"`
	NativeID   string `json:"native_id"`
	Location   string `json:"location"`
	BlobID     string `json:"blob_id"`
}
type searchJSONEvidence struct {
	Kind          string   `json:"kind"`
	ArticleSHA256 string   `json:"article_sha256"`
	ArticleBytes  int64    `json:"article_bytes"`
	FirstBlock    int      `json:"first_block"`
	AfterBlock    int      `json:"after_block"`
	Quote         string   `json:"quote"`
	Headings      []string `json:"headings"`
	NoticeLabels  []string `json:"notice_labels"`
}

func renderSearchJSON(ctx context.Context, r *publishedsearch.Reader, target string, selection publishedsearch.Selection, termsVersion string, batch searchBatch, modelRank bool, maxBytes int) ([]byte, error) {
	if len(batch.Evidence) != len(batch.Quotes.Hits) {
		return nil, publishedsearch.ErrMismatch
	}
	out := searchJSON{Format: searchJSONFormat, Status: "ok", Target: target, Selection: searchJSONSelection{selection.Collection, selection.Generation, selection.Edition}, Retrieval: searchJSONRetrieval{Policy: "all_terms_bm25", TermsVersion: termsVersion, RankingVersion: batch.Quotes.RankingVersion, IndexedMatches: batch.Quotes.TotalMatches, RankedCandidates: batch.RankedCandidates}, Results: make([]searchJSONPassage, 0, len(batch.Quotes.Hits))}
	if modelRank {
		out.Retrieval.Policy = "any_terms_bm25_then_bge"
		out.Retrieval.Model = &searchJSONModel{rerank.Model, rerank.Revision, rerank.InputVersion, rerank.Runtime}
	}
	textLeft := maxBytes
	for i, hit := range batch.Quotes.Hits {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		loc, err := r.Location(ctx, hit.Ref)
		if err != nil {
			return nil, err
		}
		e := batch.Evidence[i]
		if loc.Selection != selection || e.Quote != hit.Quote {
			return nil, publishedsearch.ErrMismatch
		}
		textLeft, err = admitJSONText(e, textLeft)
		if err != nil {
			return nil, err
		}
		d := loc.Document
		// Borrow the owned, unchanged context slices through Encode. Normalize
		// absent context to [] without copying nonempty slice storage.
		headings, labels := e.Headings, e.Labels
		if headings == nil {
			headings = []string{}
		}
		if labels == nil {
			labels = []string{}
		}
		ref, err := r.SourceReference(ctx, hit.Ref)
		if err != nil {
			return nil, err
		}
		token, err := encodeSourceReference(ref)
		if err != nil {
			return nil, err
		}
		out.Results = append(out.Results, searchJSONPassage{
			SourceRef: token,
			ID:        "r" + strconv.Itoa(i+1),
			Source:    searchJSONSource{Kind: "git", ID: d.Source.ID, Repository: d.Source.Repository, Revision: d.Revision, NativeID: d.NativeID, Location: d.Location, BlobID: d.Input.BlobID},
			Evidence:  searchJSONEvidence{Kind: "retained_html_blocks/v1", ArticleSHA256: hex.EncodeToString(d.Article.SHA256[:]), ArticleBytes: d.Article.Bytes, FirstBlock: loc.Body.First, AfterBlock: loc.Body.After, Quote: e.Quote, Headings: headings, NoticeLabels: labels},
		})
	}
	// Encoder escapes controls and HTML-sensitive text and adds LF. Its scratch
	// can exceed maxBytes; only the admitted complete output buffer is bounded.
	b := displayBuffer{max: maxBytes}
	if err := json.NewEncoder(&b).Encode(out); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return b.body, nil
}
