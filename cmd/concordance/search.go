package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/sidsharma96/concordance/internal/catalog"
	"github.com/sidsharma96/concordance/internal/generationmanifest"
	"github.com/sidsharma96/concordance/internal/htmlarticlejson"
	"github.com/sidsharma96/concordance/internal/htmledition"
	"github.com/sidsharma96/concordance/internal/objectstore"
	"github.com/sidsharma96/concordance/internal/publishedsearch"
	"github.com/sidsharma96/concordance/internal/searchtarget"
	"github.com/sidsharma96/concordance/internal/termsearch"
	"github.com/sidsharma96/concordance/internal/textterms"
)

var errOutputLimit = errors.New("formatted output limit exceeded")

// commandError retains a cause for classification without displaying driver,
// provider, query or credential-bearing diagnostics at the terminal boundary.
type commandError struct {
	stage string
	cause error
}

func (e *commandError) Error() string { return e.stage }
func (e *commandError) Unwrap() error { return e.cause }
func commandFailure(stage string, err error) error {
	var postgresError *pgconn.PgError
	switch {
	case errors.Is(err, context.Canceled):
		stage += ": canceled"
	case errors.Is(err, context.DeadlineExceeded):
		stage += ": deadline exceeded"
	case errors.Is(err, errOutputLimit):
		stage += ": increase --max-output-bytes"
	case errors.Is(err, catalog.ErrTargetOutcomeUnknown):
		stage += ": outcome unknown; retain the exact activation flags and inspect target show"
	case errors.Is(err, catalog.ErrTargetConflict):
		stage += ": target revision conflict; inspect target show"
	case errors.Is(err, catalog.ErrNotFound):
		stage += ": target, published generation or edition not found"
	case errors.Is(err, objectstore.ErrNotFound):
		stage += ": saved object not found"
	case errors.Is(err, objectstore.ErrIntegrity):
		stage += ": saved object failed integrity checks"
	case errors.Is(err, termsearch.ErrIncompatible):
		stage += ": query preparation is incompatible with the saved index"
	case errors.As(err, &postgresError) && postgresError.Code == "42P01":
		stage += ": catalog tables are missing; complete operator schema setup"
	case errors.As(err, &postgresError) && postgresError.Code == "42501":
		stage += ": catalog permission denied"
	case errors.Is(err, publishedsearch.ErrLimit), errors.Is(err, generationmanifest.ErrLimit), errors.Is(err, htmledition.ErrLimit), errors.Is(err, htmlarticlejson.ErrLimit), errors.Is(err, termsearch.ErrLimit), errors.Is(err, textterms.ErrLimit):
		stage += ": admission limit exceeded; see search --help"
	}
	return &commandError{stage, err}
}

type searchStores struct {
	catalog *catalog.Store
	objects *objectstore.Store
	close   func()
}

// The concrete connection function is the command's only test substitution.
// A successful connector transfers client ownership and supplies cleanup.
type searchConnector func(context.Context, searchConfig, int64) (searchStores, error)

type readOptions struct {
	manifest        int
	index           int
	article         int
	editionArticles int
	documents       int
	passages        int
	terms           int
	prepared        int
	timeout         time.Duration
}

func (o *readOptions) flags(f *flag.FlagSet) {
	f.DurationVar(&o.timeout, "timeout", time.Minute, "whole operation deadline")
	f.IntVar(&o.manifest, "max-manifest-bytes", 8<<20, "encoded generation manifest byte ceiling")
	f.IntVar(&o.index, "max-index-bytes", 8<<20, "encoded selected-edition index byte ceiling")
	f.IntVar(&o.article, "max-article-bytes", 8<<20, "one encoded quoted article byte ceiling")
	f.IntVar(&o.editionArticles, "max-edition-article-bytes", 64<<20, "sum of declared article bytes in the selected edition")
	f.IntVar(&o.documents, "max-documents", 10000, "manifest document count and edition article count ceiling")
	f.IntVar(&o.passages, "max-passages", 20000, "selected edition passage count ceiling")
	f.IntVar(&o.terms, "max-terms", 1000000, "selected edition term occurrence/posting ceiling")
	f.IntVar(&o.prepared, "max-prepared-bytes", 16<<20, "selected edition prepared-term byte admission ceiling")
}
func (o readOptions) valid() bool {
	for _, n := range []int{o.manifest, o.index, o.article, o.editionArticles, o.documents, o.passages, o.terms, o.prepared} {
		if n <= 0 || n == int(^uint(0)>>1) {
			return false
		}
	}
	return o.timeout > 0
}
func (o readOptions) limits() publishedsearch.Limits {
	return publishedsearch.Limits{
		Manifest: generationmanifest.Limits{
			MaxEncodedBytes: o.manifest, MaxSources: 64, MaxEditions: 64, MaxCaptures: 4096,
			MaxFiles: 20000, MaxDocuments: o.documents, MaxImages: 20000,
			MaxSourceFiles: 20000, MaxSourceDocuments: o.documents,
			MaxStringBytes: 8192, MaxValueBytes: o.manifest,
			MaxObjectBytes: 1 << 40, MaxReferencedBytes: 1 << 42,
		},
		Edition: htmledition.Limits{
			MaxEncodedBytes: o.index, MaxArticles: o.documents, MaxArticleBytes: o.editionArticles,
			MaxBlocks: 200000, MaxPassages: o.passages, MaxTerms: o.terms,
			MaxPreparedBytes: o.prepared, MaxKeyBytes: 2 << 20, MaxTermBytes: 64 << 10,
			MaxVocabulary: 20000, MaxPostings: o.terms,
		},
		Article: htmlarticlejson.Limits{
			MaxEncodedBytes: o.article, MaxBlocks: 200000, MaxGroups: 200000,
			MaxStringBytes: o.article, MaxValueBytes: o.article,
			MaxGroupDepth: 512, MaxHTMLBytes: o.article,
		},
	}
}
func validName(s string) bool {
	return s != "" && len(s) <= 256 && utf8.ValidString(s) && !strings.ContainsRune(s, 0)
}

func searchHelp(f *flag.FlagSet, out io.Writer, usage string) error {
	var b bytes.Buffer
	fmt.Fprintln(&b, usage)
	fmt.Fprintln(&b, "Flags precede positional arguments.")
	f.SetOutput(&b)
	f.PrintDefaults()
	return writeCommand(out, b.Bytes())
}
func writeCommand(out io.Writer, body []byte) error {
	if out == nil {
		return errors.New("missing command output")
	}
	n, err := out.Write(body)
	if err == nil && n != len(body) {
		err = io.ErrShortWrite
	}
	if err != nil {
		return commandFailure("write command output failed", err)
	}
	return nil
}

func runSearch(parent context.Context, args []string, out io.Writer, getenv func(string) string, connect searchConnector) error {
	if parent == nil || out == nil || getenv == nil || connect == nil || len(args) > 64 {
		return errors.New("invalid search invocation")
	}
	f := flag.NewFlagSet("search", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	var read readOptions
	read.flags(f)
	edition := f.String("edition", "", "exact edition; omitted uses the target's configured default")
	queryBytes := f.Int("max-query-bytes", 4096, "raw query UTF-8 byte ceiling")
	queryTerms := f.Int("max-query-terms", 64, "query term occurrence ceiling")
	matches := f.Int("max-matches", 1000, "complete match ceiling; excess is an error, not top-k")
	quote := f.Int("max-quote-bytes", 64<<10, "individual quote text byte ceiling")
	total := f.Int("max-total-quote-bytes", 8<<20, "sum of returned quote text bytes")
	output := f.Int("max-output-bytes", 16<<20, "fully formatted display byte ceiling")
	if err := f.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return searchHelp(f, out, "Usage: concordance search [flags] TARGET \"QUERY\"")
		}
		return commandFailure("invalid search flags; use search --help", err)
	}
	if f.NArg() != 2 || !validName(f.Arg(0)) || !read.valid() || *queryBytes <= 0 || *queryTerms <= 0 || *matches <= 0 || *quote <= 0 || *total <= 0 || *output <= 0 {
		return errors.New("provide a target, one quoted query and positive search limits; use search --help")
	}
	editionSet := false
	f.Visit(func(v *flag.Flag) {
		if v.Name == "edition" {
			editionSet = true
		}
	})
	if editionSet && !validName(*edition) {
		return errors.New("provide a nonempty exact edition or omit --edition")
	}
	ctx, cancel := context.WithTimeout(parent, read.timeout)
	defer cancel()
	// Query admission precedes connection configuration and all catalog/object I/O.
	prepared, err := textterms.Prepare(ctx, f.Arg(1), textterms.Limits{MaxInputBytes: *queryBytes, MaxTerms: *queryTerms, MaxTermBytes: *queryBytes, MaxOutputBytes: *queryBytes})
	if err != nil {
		return commandFailure("prepare query failed", err)
	}
	if len(prepared.Terms) == 0 {
		return errors.New("query must contain at least one technical term")
	}
	cfg, err := loadSearchConfig(getenv, true)
	if err != nil {
		return err
	}
	stores, err := connect(ctx, cfg, int64(max(read.manifest, read.index, read.article)))
	if err != nil {
		return err
	}
	defer stores.close()
	selection, err := searchtarget.Resolve(ctx, stores.catalog, f.Arg(0), *edition)
	if err != nil {
		return commandFailure("resolve search target failed", err)
	}
	if selection.Collection != cfg.collection {
		return errors.New("target is outside CONCORDANCE_COLLECTION; no object read was attempted")
	}
	r, err := publishedsearch.Open(ctx, stores.catalog, stores.objects, selection, read.limits())
	if err != nil {
		return commandFailure("open published selection failed", err)
	}
	q := termsearch.Query{Edition: selection.Edition, TermsVersion: prepared.Version, Terms: prepared.Terms}
	batch, err := r.SearchQuoted(ctx, q, termsearch.Limits{MaxEditions: 1, MaxPassages: read.passages, MaxTerms: read.terms, MaxQueryTerms: *queryTerms, MaxInputBytes: read.prepared, MaxMatches: *matches}, publishedsearch.QuoteLimits{MaxQuoteBytes: *quote, MaxTotalQuoteBytes: *total})
	if err != nil {
		return commandFailure("collect verified search quotes failed", err)
	}
	body, err := renderSearch(ctx, r, f.Arg(0), selection, batch, *output)
	if err != nil {
		return commandFailure("format search results failed", err)
	}
	if err := ctx.Err(); err != nil {
		return commandFailure("search failed before output", err)
	}
	return writeCommand(out, body)
}
