package htmlarticle

import (
	"context"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/net/html"
)

type extractor struct {
	ctx     context.Context
	profile Profile
	limits  Limits
	article Article
	used    int
	text    strings.Builder
	// panelLabels maps each validated tab panel to its tab's folded text.
	panelLabels map[*html.Node]string
	space       bool
	anchor      string
}

func (w *extractor) reserve(n int) error {
	if n < 0 || n > w.limits.MaxOutputBytes-w.used {
		return ErrLimit
	}
	w.used += n
	return nil
}

func (w *extractor) appendBlock(b Block, chargedText bool) error {
	if len(w.article.Blocks) >= w.limits.MaxBlocks || len(b.Text) > w.limits.MaxBlockBytes {
		return ErrLimit
	}
	n := len(b.Anchor) + len(b.SourceURL)
	if !chargedText {
		n += len(b.Text)
	}
	if err := w.reserve(n); err != nil {
		return err
	}
	b.Anchor, b.SourceURL = strings.Clone(b.Anchor), strings.Clone(b.SourceURL)
	if !chargedText {
		b.Text = strings.Clone(b.Text)
	}
	w.article.Blocks = append(w.article.Blocks, b)
	return nil
}

func (w *extractor) write(text string, anchor string) error {
	for _, c := range text {
		if unicode.IsSpace(c) {
			w.space = true
			continue
		}
		n := utf8.RuneLen(c)
		if w.space && w.text.Len() > 0 {
			n++
		}
		if n > w.limits.MaxBlockBytes-w.text.Len() {
			return ErrLimit
		}
		if err := w.reserve(n); err != nil {
			return err
		}
		if w.text.Len() == 0 {
			w.anchor = anchor
		}
		if w.space && w.text.Len() > 0 {
			w.text.WriteByte(' ')
		}
		w.space = false
		w.text.WriteRune(c)
	}
	return nil
}

func (w *extractor) flush() error { return w.finish(Prose, 0, "", false) }

func (w *extractor) finish(kind Kind, level int, anchor string, force bool) error {
	if w.text.Len() == 0 && !force {
		w.space = false
		return nil
	}
	if anchor == "" {
		anchor = w.anchor
	}
	err := w.appendBlock(Block{Kind: kind, Text: w.text.String(), Level: level, Anchor: anchor}, true)
	w.text.Reset()
	w.space, w.anchor = false, ""
	return err
}

func (w *extractor) omit(n *html.Node) bool {
	for _, id := range w.profile.OmitIDs {
		if attribute(n, "id") == id {
			return true
		}
	}
	for _, class := range w.profile.OmitClasses {
		if hasClass(n, class) {
			return true
		}
	}
	return false
}

func (w *extractor) children(n *html.Node, anchor string) error {
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if err := w.walk(child, anchor); err != nil {
			return err
		}
	}
	return nil
}

func (w *extractor) container(n *html.Node, anchor string) error {
	if err := w.flush(); err != nil {
		return err
	}
	if err := w.children(n, anchor); err != nil {
		return err
	}
	return w.flush()
}

func (w *extractor) grouped(n *html.Node, anchor string, group Group) error {
	if err := w.flush(); err != nil {
		return err
	}
	if len(w.article.Groups) >= w.limits.MaxGroups {
		return ErrLimit
	}
	if err := w.reserve(len(group.Label)); err != nil {
		return err
	}
	group.Label = strings.Clone(group.Label)
	group.First = len(w.article.Blocks)
	index := len(w.article.Groups)
	w.article.Groups = append(w.article.Groups, group)
	if err := w.children(n, anchor); err != nil {
		return err
	}
	if err := w.flush(); err != nil {
		return err
	}
	w.article.Groups[index].After = len(w.article.Blocks)
	return nil
}

func (w *extractor) walk(n *html.Node, anchor string) error {
	if err := w.ctx.Err(); err != nil {
		return err
	}
	switch n.Type {
	case html.TextNode:
		return w.write(n.Data, anchor)
	case html.CommentNode:
		return nil
	case html.ElementNode:
	default:
		return ErrUnsupported
	}
	if n.Namespace != "" {
		return ErrUnsupported
	}
	if w.omit(n) {
		return nil
	}
	if id := attribute(n, "id"); id != "" {
		anchor = id
	}
	switch role := attribute(n, "role"); {
	case w.panelLabels == nil:
		// No button tabs: roles keep their earlier meaning.
	case role == "tablist":
		// collectTabs checked that the list holds only tabs; their text now
		// labels the matching panels.
		return nil
	case role == "tabpanel":
		label, ok := w.panelLabels[n]
		if !ok {
			return ErrUnsupported
		}
		return w.grouped(n, anchor, Group{Kind: Notice, Label: label})
	}
	switch n.Data {
	case "script", "style", "template", "meta", "link":
		return nil
	case "h1", "h2", "h3", "h4", "h5", "h6":
		if err := w.flush(); err != nil {
			return err
		}
		if err := w.inline(n, anchor, false); err != nil {
			return err
		}
		return w.finish(Heading, int(n.Data[1]-'0'), anchor, true)
	case "pre":
		if err := w.flush(); err != nil {
			return err
		}
		if err := w.inline(n, anchor, true); err != nil {
			return err
		}
		return w.finish(Code, 0, anchor, true)
	case "p":
		return w.grouped(n, anchor, Group{Kind: Paragraph})
	case "ul", "ol":
		if hasAttribute(n, "reversed") ||
			attribute(n, "start") != "" && attribute(n, "start") != "1" ||
			attribute(n, "type") != "" && attribute(n, "type") != "1" {
			return ErrUnsupported
		}
		return w.grouped(n, anchor, Group{Kind: List, Ordered: n.Data == "ol"})
	case "li":
		if hasAttribute(n, "value") {
			return ErrUnsupported
		}
		return w.grouped(n, anchor, Group{Kind: ListItem})
	case "dl":
		return w.grouped(n, anchor, Group{Kind: DefinitionList})
	case "dt":
		return w.grouped(n, anchor, Group{Kind: Term})
	case "dd":
		return w.grouped(n, anchor, Group{Kind: Definition})
	case "table":
		return w.grouped(n, anchor, Group{Kind: Table})
	case "tr":
		return w.grouped(n, anchor, Group{Kind: Row})
	case "th", "td":
		for _, key := range []string{"rowspan", "colspan"} {
			if value := attribute(n, key); value != "" && value != "1" {
				return ErrUnsupported
			}
		}
		return w.grouped(n, anchor, Group{Kind: Cell, Header: n.Data == "th"})
	case "figure":
		return w.grouped(n, anchor, Group{Kind: Figure})
	case "caption", "figcaption":
		return w.grouped(n, anchor, Group{Kind: Caption})
	case "blockquote":
		return w.grouped(n, anchor, Group{Kind: Quotation})
	case "img":
		if err := w.flush(); err != nil {
			return err
		}
		return w.appendBlock(Block{Kind: Image, Text: attribute(n, "alt"), Anchor: anchor, SourceURL: attribute(n, "src")}, false)
	case "hr":
		if err := w.flush(); err != nil {
			return err
		}
		return w.appendBlock(Block{Kind: Separator, Anchor: anchor}, false)
	case "br":
		w.space = true
		return nil
	case "div":
		for _, class := range w.profile.NoticeClasses {
			if hasClass(n, class) {
				return w.grouped(n, anchor, Group{Kind: Notice, Label: attribute(n, "title")})
			}
		}
		return w.container(n, anchor)
	case "section", "article", "main", "header", "footer", "aside", "thead", "tbody", "tfoot":
		return w.container(n, anchor)
	default:
		if inlineTag(n.Data) {
			return w.children(n, anchor)
		}
		return ErrUnsupported
	}
}

// inline collects one heading/code block without flattening nested block elements.
func (w *extractor) inline(n *html.Node, anchor string, code bool) error {
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if err := w.ctx.Err(); err != nil {
			return err
		}
		if child.Type == html.CommentNode {
			continue
		}
		if child.Type == html.TextNode {
			if !code {
				if err := w.write(child.Data, anchor); err != nil {
					return err
				}
			} else {
				if len(child.Data) > w.limits.MaxBlockBytes-w.text.Len() {
					return ErrLimit
				}
				if err := w.reserve(len(child.Data)); err != nil {
					return err
				}
				w.text.WriteString(child.Data)
			}
			continue
		}
		if child.Type != html.ElementNode || child.Namespace != "" {
			return ErrUnsupported
		}
		if w.omit(child) {
			continue
		}
		if child.Data == "br" {
			if code {
				if w.text.Len() >= w.limits.MaxBlockBytes {
					return ErrLimit
				}
				if err := w.reserve(1); err != nil {
					return err
				}
				w.text.WriteByte('\n')
			} else {
				w.space = true
			}
			continue
		}
		if !inlineTag(child.Data) {
			return ErrUnsupported
		}
		if err := w.inline(child, anchor, code); err != nil {
			return err
		}
	}
	return nil
}

func inlineTag(tag string) bool {
	switch tag {
	case "a", "span", "code", "em", "strong", "b", "i", "u", "s", "small", "abbr", "kbd", "samp", "var", "mark", "sup", "sub", "time", "cite", "q", "del", "ins", "dfn", "bdi", "bdo", "wbr":
		return true
	}
	return false
}

func hasAttribute(n *html.Node, key string) bool {
	for _, a := range n.Attr {
		if a.Namespace == "" && a.Key == key {
			return true
		}
	}
	return false
}
