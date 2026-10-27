// Command corpusaudit inventories bounded committed Markdown without extracting
// passages or rendering a site. Marker counts are lexical clues, not parser coverage.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/sidsharma96/concordance/internal/gitsource"
)

var (
	shortcode = regexp.MustCompile(`\{\{[<%]\s*([A-Za-z][A-Za-z0-9_-]*)`)
	htmlLike  = regexp.MustCompile(`<[A-Za-z][A-Za-z0-9]*[\s>]`)
)

type inventory struct {
	documents                               []gitsource.Document
	bytes, uniqueBytes, largestBytes        int64
	unique, frontmatter, shortcodes, images int
	html                                    int
	markerDocuments                         map[string]int
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, out io.Writer) error {
	f := flag.NewFlagSet("corpusaudit", flag.ContinueOnError)
	var request gitsource.Request
	var limits gitsource.Limits
	f.StringVar(&request.Repository, "repo", "", "trusted local repository (already acquired)")
	f.StringVar(&request.Commit, "commit", "", "full immutable commit ID")
	f.StringVar(&request.Directory, "dir", "", "repository-relative selected directory")
	// Audit ceilings only, not production importer limits or measured capacity.
	f.IntVar(&limits.MaxEntries, "max-entries", 20000, "maximum enumerated entries")
	f.Int64Var(&limits.MaxListingBytes, "max-listing-bytes", 8<<20, "maximum Git listing bytes")
	f.Int64Var(&limits.MaxDocumentBytes, "max-document-bytes", 4<<20, "maximum selected document bytes")
	f.Int64Var(&limits.MaxTotalBytes, "max-total-bytes", 128<<20, "maximum selected body bytes")
	timeout := f.Duration("timeout", 2*time.Minute, "capture deadline, also checked before report output")
	detailed := f.Bool("inventory", false, "include sorted document paths, SHA-256 and lengths")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 || *timeout <= 0 {
		return fmt.Errorf("use named flags and a positive timeout")
	}
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	snapshot, err := gitsource.Capture(ctx, request, limits)
	if err != nil {
		return fmt.Errorf("capture corpus: %w", err)
	}
	stats := inspect(snapshot)
	if err := ctx.Err(); err != nil {
		return err
	}
	var report strings.Builder
	fmt.Fprintf(&report, "commit\t%s\ndirectory\t%s\n", snapshot.Commit, snapshot.Directory)
	fmt.Fprintf(&report, "documents\t%d\nignored_entries\t%d\nbody_bytes\t%d\n", len(stats.documents), snapshot.Ignored, stats.bytes)
	fmt.Fprintf(&report, "unique_bodies\t%d\nunique_body_bytes\t%d\nlargest_body_bytes\t%d\n", stats.unique, stats.uniqueBytes, stats.largestBytes)
	fmt.Fprintf(&report, "frontmatter_marker_docs\t%d\nshortcode_marker_docs\t%d\nmarkdown_image_marker_docs\t%d\nhtml_like_marker_docs\t%d\n",
		stats.frontmatter, stats.shortcodes, stats.images, stats.html)
	names := make([]string, 0, len(stats.markerDocuments))
	for name := range stats.markerDocuments {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		fmt.Fprintf(&report, "shortcode_name_docs\t%s\t%d\n", name, stats.markerDocuments[name])
	}
	if *detailed {
		for _, document := range stats.documents {
			fmt.Fprintf(&report, "document\t%q\t%x\t%d\n", document.Path, document.SHA256, len(document.Body))
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	_, err = io.WriteString(out, report.String())
	return err
}

// inspect borrows immutable bodies. It sorts a copy of their descriptors and owns
// its counts/maps; it neither modifies nor copies the original document bodies.
func inspect(snapshot gitsource.Snapshot) inventory {
	stats := inventory{documents: slices.Clone(snapshot.Documents), markerDocuments: make(map[string]int)}
	slices.SortFunc(stats.documents, func(a, b gitsource.Document) int { return strings.Compare(a.Path, b.Path) })
	seen := make(map[[sha256.Size]byte]bool)
	for _, document := range stats.documents {
		body := document.Body
		size := int64(len(body))
		stats.bytes += size
		stats.largestBytes = max(stats.largestBytes, size)
		if !seen[document.SHA256] {
			seen[document.SHA256] = true
			stats.unique++
			stats.uniqueBytes += size
		}
		if bytes.HasPrefix(body, []byte("---\n")) || bytes.HasPrefix(body, []byte("---\r\n")) ||
			bytes.HasPrefix(body, []byte("+++\n")) || bytes.HasPrefix(body, []byte("+++\r\n")) {
			stats.frontmatter++
		}
		if bytes.Contains(body, []byte("{{<")) || bytes.Contains(body, []byte("{{%")) {
			stats.shortcodes++
		}
		if bytes.Contains(body, []byte("![")) {
			stats.images++
		}
		if htmlLike.Match(body) {
			stats.html++
		}
		names := make(map[string]bool)
		for _, match := range shortcode.FindAllSubmatch(body, -1) {
			names[string(match[1])] = true
		}
		for name := range names {
			stats.markerDocuments[name]++
		}
	}
	return stats
}
