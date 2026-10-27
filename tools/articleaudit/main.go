// Command articleaudit inspects one retained Kubernetes reference article.
// It does not build a website, acquire dependencies, fetch images or publish data.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/sidsharma96/concordance/internal/htmlarticle"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func referenceProfile() htmlarticle.Profile {
	return htmlarticle.Profile{
		ID: "kubernetes-pods-reference-v1", ArticleClass: "td-content",
		OmitClasses: []string{"article-meta", "copy-code-icon", "icon-copycode", "section-index", "td-heading-self-link"},
		OmitIDs:     []string{"pre-footer"}, NoticeClasses: []string{"feature-state-notice", "alert", "pageinfo", "callout"},
	}
}

func run(ctx context.Context, args []string, out io.Writer) error {
	f := flag.NewFlagSet("articleaudit", flag.ContinueOnError)
	file := f.String("file", "", "trusted, stable, retained Kubernetes article HTML file")
	maxInput := f.Int("max-input-bytes", 4<<20, "input byte ceiling for this audit, not a product limit")
	timeout := f.Duration("timeout", 15*time.Second, "extraction deadline; regular file reads are not preemptive")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 || *file == "" || *maxInput <= 0 || *maxInput == int(^uint(0)>>1) || *timeout <= 0 {
		return errors.New("provide -file and positive audit limits")
	}
	if filepath.Base(filepath.Dir(filepath.Clean(*file))) == "_print" {
		return errors.New("exclude the generated _print aggregate from this audit")
	}
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return err
	}
	body, err := readFile(*file, *maxInput)
	if err != nil {
		return err
	}
	limits := htmlarticle.Limits{MaxInputBytes: *maxInput, MaxTokenBytes: min(*maxInput, 128<<10),
		MaxTokens: 200000, MaxNodes: 200000, MaxDepth: 512, MaxBlocks: 20000, MaxGroups: 40000,
		MaxBlockBytes: 1 << 20, MaxOutputBytes: 8 << 20}
	a, err := htmlarticle.Extract(ctx, body, referenceProfile(), limits)
	if err != nil {
		return fmt.Errorf("extract retained article: %w", err)
	}
	counts := make(map[htmlarticle.Kind]int)
	textBytes := 0
	for _, b := range a.Blocks {
		counts[b.Kind]++
		textBytes += len(b.Text)
	}
	var report strings.Builder
	fmt.Fprintf(&report, "processor\t%s\nprofile\t%s\nhtml_sha256\t%x\nhtml_bytes\t%d\n", a.Processor, a.Profile, a.HTMLSHA256, a.HTMLBytes)
	fmt.Fprintf(&report, "blocks\t%d\ngroups\t%d\nblock_text_bytes\t%d\nheadings\t%d\ncode_blocks\t%d\nimage_references\t%d\n", len(a.Blocks), len(a.Groups), textBytes, counts[htmlarticle.Heading], counts[htmlarticle.Code], counts[htmlarticle.Image])
	report.WriteString("image_content_interpreted\tfalse\n")
	if err := ctx.Err(); err != nil {
		return err
	}
	_, err = io.WriteString(out, report.String())
	return err
}

// The caller owns a trusted, stable local file. The checks reject a final symlink
// or nonregular file; they do not sandbox hostile concurrent filesystem changes.
func readFile(name string, maxBytes int) (body []byte, resultErr error) {
	info, err := os.Lstat(name)
	if err != nil {
		return nil, fmt.Errorf("inspect article input: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() > int64(maxBytes) {
		return nil, errors.New("article input must be a regular file within the byte ceiling")
	}
	f, err := os.Open(name)
	if err != nil {
		return nil, fmt.Errorf("open article input: %w", err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			body = nil
			resultErr = errors.Join(resultErr, err)
		}
	}()
	opened, err := f.Stat()
	if err != nil || !opened.Mode().IsRegular() {
		return nil, errors.Join(errors.New("article input is not a stable regular file"), err)
	}
	body, err = io.ReadAll(io.LimitReader(f, int64(maxBytes)+1))
	if err != nil {
		return nil, fmt.Errorf("read article input: %w", err)
	}
	if len(body) > maxBytes {
		return nil, htmlarticle.ErrLimit
	}
	return body, nil
}
