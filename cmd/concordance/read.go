package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"io"

	"github.com/sidsharma96/concordance/internal/htmlpassage"
	"github.com/sidsharma96/concordance/internal/publishedsearch"
)

func runRead(parent context.Context, args []string, out io.Writer, getenv func(string) string, connect searchConnector) error {
	if parent == nil || out == nil || getenv == nil || connect == nil || len(args) > 64 {
		return errors.New("invalid read invocation")
	}
	f := flag.NewFlagSet("read", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	var read readOptions
	read.flags(f)
	scope := f.String("scope", "exact", "exact, section, parent, previous or next; ranges keep whole authored passages")
	textBytes := f.Int("max-read-bytes", 64<<10, "returned body UTF-8 byte ceiling; no truncation")
	outputBytes := f.Int("max-output-bytes", 256<<10, "complete encoded JSON byte ceiling")
	if err := f.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return searchHelp(f, out, "Usage: concordance read [flags] SOURCE_REF\nUse --scope section to inspect a search result; follow navigation references with the default exact scope.\nReferences pin captured sources, not permission. Output is versioned JSON.")
		}
		return commandFailure("invalid read flags; use read --help", err)
	}
	if f.NArg() != 1 || !read.valid() || *textBytes <= 0 || *outputBytes <= 0 {
		return errors.New("provide one source reference and positive limits; use read --help")
	}
	switch htmlpassage.Scope(*scope) {
	case htmlpassage.Exact, htmlpassage.Section, htmlpassage.Parent, htmlpassage.Previous, htmlpassage.Next:
	default:
		return errors.New("unknown read scope; use read --help")
	}
	ref, err := decodeSourceReference(f.Arg(0))
	if err != nil {
		return err
	}
	cfg, err := loadSearchConfig(getenv, true)
	if err != nil {
		return err
	}
	if ref.Selection.Collection != cfg.collection {
		return errors.New("reference is outside CONCORDANCE_COLLECTION; no connection or object read was attempted")
	}
	ctx, cancel := context.WithTimeout(parent, read.timeout)
	defer cancel()
	stores, err := connect(ctx, cfg, int64(max(read.manifest, read.index, read.article)))
	if err != nil {
		return err
	}
	defer stores.close()
	r, err := publishedsearch.Open(ctx, stores.catalog, stores.objects, ref.Selection, read.limits())
	if err != nil {
		return commandFailure("open captured source failed", err)
	}
	result, err := r.ReadSource(ctx, ref, htmlpassage.Scope(*scope), evidenceLimits(read, *textBytes))
	if err != nil {
		if errors.Is(err, publishedsearch.ErrLimit) || errors.Is(err, htmlpassage.ErrLimit) {
			return &commandError{stage: "captured-source read exceeds admission limits; use --scope exact/previous/next or raise the relevant read limit", cause: err}
		}
		if errors.Is(err, htmlpassage.ErrNoContext) {
			return errors.New("no surrounding source in that direction; use returned navigation references")
		}
		return commandFailure("read captured source failed", err)
	}
	body, err := renderSourceJSON(ctx, result, *outputBytes)
	if err != nil {
		return commandFailure("format captured source failed", err)
	}
	if err := ctx.Err(); err != nil {
		return commandFailure("read failed before output", err)
	}
	return writeCommand(out, body)
}

type readNoticeJSON struct {
	First int    `json:"first_block"`
	After int    `json:"after_block"`
	Label string `json:"label"`
}
type readNavigationJSON struct {
	Previous *string `json:"previous"`
	Next     *string `json:"next"`
	Parent   *string `json:"parent"`
}
type readJSON struct {
	Format     string              `json:"format"`
	Status     string              `json:"status"`
	Selection  searchJSONSelection `json:"selection"`
	Source     searchJSONSource    `json:"source"`
	SourceRef  string              `json:"source_ref"`
	Evidence   searchJSONEvidence  `json:"evidence"`
	Notices    []readNoticeJSON    `json:"notices"`
	Navigation readNavigationJSON  `json:"navigation"`
}

func renderSourceJSON(ctx context.Context, r publishedsearch.SourceRead, maxBytes int) ([]byte, error) {
	ref, err := encodeSourceReference(r.Reference)
	if err != nil {
		return nil, err
	}
	nav := make([]*string, 3)
	for i, n := range []*publishedsearch.SourceReference{r.Previous, r.Next, r.Parent} {
		if n == nil {
			continue
		}
		s, err := encodeSourceReference(*n)
		if err != nil {
			return nil, err
		}
		nav[i] = &s
	}
	loc, d := r.Location, r.Location.Document
	out := readJSON{
		Format: "concordance.docs-read/v1", Status: "ok",
		Selection: searchJSONSelection{loc.Selection.Collection, loc.Selection.Generation, loc.Selection.Edition},
		Source: searchJSONSource{
			Kind: "git", ID: d.Source.ID, Repository: d.Source.Repository, Revision: d.Revision,
			NativeID: d.NativeID, Location: d.Location, BlobID: d.Input.BlobID,
		},
		SourceRef: ref,
		Evidence: searchJSONEvidence{
			Kind: "retained_html_blocks/v1", ArticleSHA256: hex.EncodeToString(d.Article.SHA256[:]), ArticleBytes: d.Article.Bytes,
			FirstBlock: loc.Body.First, AfterBlock: loc.Body.After, Quote: r.Quote,
			Headings: append([]string{}, r.Headings...), NoticeLabels: []string{},
		},
		Notices:    make([]readNoticeJSON, 0, len(r.Notices)),
		Navigation: readNavigationJSON{nav[0], nav[1], nav[2]},
	}
	for _, n := range r.Notices {
		out.Notices = append(out.Notices, readNoticeJSON{n.Body.First, n.Body.After, n.Label})
	}
	b := displayBuffer{max: maxBytes}
	if err := json.NewEncoder(&b).Encode(out); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return b.body, nil
}
