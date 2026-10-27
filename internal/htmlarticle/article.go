// Package htmlarticle extracts bounded, in-memory structure from captured HTML.
// It does not fetch resources, execute HTML, map Git lines, choose search passages
// or publish a generation. Images remain references, not interpreted content.
package htmlarticle

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html"
)

var (
	ErrInvalid     = errors.New("invalid HTML extraction request")
	ErrLimit       = errors.New("HTML extraction limit exceeded")
	ErrSelection   = errors.New("expected exactly one HTML article")
	ErrUnsupported = errors.New("unsupported HTML article structure")
)

const Processor = "htmlarticle/v1"

// Profile contains exact, case-sensitive HTML class/ID tokens, not CSS selectors.
// Its ID versions the caller's selection/omission policy. Changing that policy
// requires a different ID. The caller must not mutate it during Extract.
type Profile struct {
	ID            string
	ArticleClass  string
	OmitClasses   []string
	OmitIDs       []string
	NoticeClasses []string
}

// Limits bound admission, traversal and retained output, not precise parser heap
// or CPU. Token admission precedes DOM allocation; node/depth checks follow it.
// MaxOutputBytes counts text, anchors, URLs, labels and processor/profile names,
// not shared enum names, slice capacity or struct overhead.
type Limits struct {
	MaxInputBytes  int
	MaxTokenBytes  int
	MaxTokens      int
	MaxNodes       int
	MaxDepth       int
	MaxBlocks      int
	MaxGroups      int
	MaxBlockBytes  int
	MaxOutputBytes int
}

type Kind string

const (
	Heading   Kind = "heading"
	Prose     Kind = "prose"
	Code      Kind = "code"
	Image     Kind = "image"
	Separator Kind = "separator"
)

// Block text is HTML-decoded. Prose whitespace is folded; code whitespace is
// preserved as parsed by HTML5 (including its newline normalization). Anchor is
// only a navigation hint. An image has alt Text and SourceURL, with no fetched
// image bytes or claim that its depicted meaning has been extracted.
type Block struct {
	Kind      Kind
	Text      string
	Level     int
	Anchor    string
	SourceURL string
}

type GroupKind string

const (
	Paragraph      GroupKind = "paragraph"
	Notice         GroupKind = "notice"
	List           GroupKind = "list"
	ListItem       GroupKind = "list-item"
	DefinitionList GroupKind = "definition-list"
	Term           GroupKind = "term"
	Definition     GroupKind = "definition"
	Table          GroupKind = "table"
	Row            GroupKind = "row"
	Cell           GroupKind = "cell"
	Figure         GroupKind = "figure"
	Caption        GroupKind = "caption"
	Quotation      GroupKind = "quotation"
)

// Group preserves a container's half-open block interval [First, After).
// Groups are in opening order, so nested groups follow their parents. Empty
// groups are allowed; a quoted passage must select actual blocks separately.
// Ordered applies to List; Header applies to Cell; Label identifies a notice.
// Nondefault list numbering and table spans are explicitly unsupported for now.
type Group struct {
	Kind         GroupKind
	First, After int
	Label        string
	Ordered      bool
	Header       bool
}

// Article and all its buffers belong to the caller. This is not a persisted
// schema. HTMLSHA256 identifies the input bytes, not this derived article, a
// logical document, its product edition or an authorization decision. Block
// positions have meaning only within this exact output and processing profile.
type Article struct {
	Processor  string
	Profile    string
	HTMLSHA256 [sha256.Size]byte
	HTMLBytes  int
	Blocks     []Block
	Groups     []Group
}

// Extract borrows body until return and returns no partial article on failure.
// The caller authorizes access and bounds concurrent calls. A deadline is required;
// cancellation is cooperative, not preemption of parser CPU between reader calls.
func Extract(ctx context.Context, body []byte, profile Profile, limits Limits) (Article, error) {
	if err := ctx.Err(); err != nil {
		return Article{}, err
	}
	if !validRequest(ctx, profile, limits) {
		return Article{}, ErrInvalid
	}
	if len(body) > limits.MaxInputBytes {
		return Article{}, ErrLimit
	}
	if !utf8.Valid(body) || bytes.IndexByte(body, 0) >= 0 {
		return Article{}, ErrInvalid
	}
	if err := admit(ctx, body, limits); err != nil {
		return Article{}, err
	}
	root, err := html.Parse(&contextReader{ctx: ctx, reader: bytes.NewReader(body)})
	if err != nil {
		if err := ctx.Err(); err != nil {
			return Article{}, err
		}
		// Do not expose parser diagnostics containing captured material.
		return Article{}, ErrUnsupported
	}
	selected, err := selectArticle(ctx, root, profile.ArticleClass, limits)
	if err != nil {
		return Article{}, err
	}
	if selected.Namespace != "" || selected.Data != "div" && selected.Data != "article" && selected.Data != "main" && selected.Data != "section" {
		return Article{}, ErrUnsupported
	}
	w := extractor{ctx: ctx, profile: profile, limits: limits}
	if w.omit(selected) {
		return Article{}, ErrSelection
	}
	if err := w.reserve(len(Processor) + len(profile.ID)); err != nil {
		return Article{}, err
	}
	w.article = Article{Processor: Processor, Profile: strings.Clone(profile.ID), HTMLSHA256: sha256.Sum256(body), HTMLBytes: len(body)}
	if err := w.children(selected, attribute(selected, "id")); err != nil {
		return Article{}, err
	}
	if err := w.flush(); err != nil {
		return Article{}, err
	}
	if err := ctx.Err(); err != nil {
		return Article{}, err
	}
	if len(w.article.Blocks) == 0 {
		return Article{}, ErrUnsupported
	}
	return w.article, nil
}

type contextReader struct {
	ctx    context.Context
	reader *bytes.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

func admit(ctx context.Context, body []byte, limits Limits) error {
	z := html.NewTokenizer(&contextReader{ctx: ctx, reader: bytes.NewReader(body)})
	z.SetMaxBuf(limits.MaxTokenBytes + 1)
	for count := 0; ; count++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if z.Next() == html.ErrorToken {
			if errors.Is(z.Err(), io.EOF) {
				return nil
			}
			if errors.Is(z.Err(), html.ErrBufferExceeded) {
				return ErrLimit
			}
			return z.Err()
		}
		if count >= limits.MaxTokens || len(z.Raw()) > limits.MaxTokenBytes {
			return ErrLimit
		}
	}
}

func selectArticle(ctx context.Context, root *html.Node, class string, limits Limits) (*html.Node, error) {
	var selected *html.Node
	nodes := 0
	var visit func(*html.Node, int) error
	visit = func(n *html.Node, depth int) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		nodes++
		if nodes > limits.MaxNodes || depth > limits.MaxDepth {
			return ErrLimit
		}
		if n.Type == html.ElementNode && hasClass(n, class) {
			if selected != nil {
				return ErrSelection
			}
			selected = n
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			if err := visit(child, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	if err := visit(root, 0); err != nil {
		return nil, err
	}
	if selected == nil {
		return nil, ErrSelection
	}
	return selected, nil
}

func validRequest(ctx context.Context, p Profile, l Limits) bool {
	_, deadline := ctx.Deadline()
	maxInt := int(^uint(0) >> 1)
	if !deadline || !token(p.ID, true) || !token(p.ArticleClass, false) ||
		l.MaxInputBytes <= 0 || l.MaxTokenBytes <= 0 || l.MaxTokenBytes >= maxInt ||
		l.MaxTokenBytes > l.MaxInputBytes || l.MaxTokens <= 0 || l.MaxNodes <= 0 ||
		l.MaxDepth <= 0 || l.MaxDepth > 512 || l.MaxBlocks <= 0 || l.MaxGroups <= 0 ||
		l.MaxBlockBytes <= 0 || l.MaxOutputBytes < l.MaxBlockBytes {
		return false
	}
	for _, values := range [][]string{p.OmitClasses, p.OmitIDs, p.NoticeClasses} {
		if len(values) > 32 {
			return false
		}
		for i, value := range values {
			if !token(value, false) {
				return false
			}
			for _, previous := range values[:i] {
				if previous == value {
					return false
				}
			}
		}
	}
	for _, omitted := range p.OmitClasses {
		if omitted == p.ArticleClass {
			return false
		}
	}
	return true
}

func token(s string, version bool) bool {
	if len(s) == 0 || len(s) > 128 {
		return false
	}
	for _, c := range s {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || version && (c == '/' || c == '.') {
			continue
		}
		return false
	}
	return true
}

func attribute(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Namespace == "" && a.Key == key {
			return a.Val
		}
	}
	return ""
}

func hasClass(n *html.Node, class string) bool {
	for part := range strings.FieldsFuncSeq(attribute(n, "class"), htmlSpace) {
		if part == class {
			return true
		}
	}
	return false
}

func htmlSpace(c rune) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' }
