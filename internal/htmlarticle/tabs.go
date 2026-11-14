package htmlarticle

import (
	"strings"
	"unicode"

	"golang.org/x/net/html"
)

// collectTabs validates WAI-ARIA tab structures inside the selected article and
// records each tab panel's label before the extraction walk.
//
// A tab list (role="tablist") holds tab buttons; each tab panel
// (role="tabpanel") names its tab with aria-labelledby. The walk omits each tab
// list and stores every panel as a Notice group whose Label is its tab's text,
// so the label stays attached to the panel's blocks instead of becoming
// detached list text.
//
// Handling starts only when the article contains a <button role="tab">. Every
// such input failed before this support existed, because <button> is not an
// accepted element, so no previously accepted article can change. Once
// started, only a complete one-to-one structure of button tabs is accepted: a
// tab outside a tab list, other text or elements in a tab list, a missing or
// duplicate ID, an unreferenced tab or an empty label fails with ErrUnsupported
// rather than silently dropping or misattributing verbatim text.
//
// Profile omissions and the walk's skipped elements apply first, as in the
// walk. Cost is O(nodes) for the article plus one O(nodes) pass over the
// document for ID uniqueness, with label bytes bounded by MaxBlockBytes each
// and charged to MaxOutputBytes when the panel group is appended.
func (w *extractor) collectTabs(root, selected *html.Node) error {
	if found, err := w.hasTabButton(selected); err != nil || !found {
		return err
	}
	var tabs, panels []*html.Node
	var visit func(n *html.Node, inList bool) error
	visit = func(n *html.Node, inList bool) error {
		if err := w.ctx.Err(); err != nil {
			return err
		}
		switch n.Type {
		case html.TextNode:
			if inList && strings.TrimFunc(n.Data, unicode.IsSpace) != "" {
				return ErrUnsupported
			}
			return nil
		case html.ElementNode:
		default:
			return nil
		}
		if w.omit(n) || skipped(n.Data) {
			return nil
		}
		switch attribute(n, "role") {
		case "tablist":
			if inList || n.Data != "ul" && n.Data != "ol" && n.Data != "div" {
				return ErrUnsupported
			}
			inList = true
		case "tab":
			if !inList || n.Data != "button" {
				return ErrUnsupported
			}
			tabs = append(tabs, n)
			return nil
		case "tabpanel":
			if inList || n.Data != "div" {
				return ErrUnsupported
			}
			panels = append(panels, n)
		default:
			if inList && n.Data != "li" {
				return ErrUnsupported
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			if err := visit(child, inList); err != nil {
				return err
			}
		}
		return nil
	}
	// The walk handles the selected container's children, not its own role.
	for child := selected.FirstChild; child != nil; child = child.NextSibling {
		if err := visit(child, false); err != nil {
			return err
		}
	}
	if len(tabs) != len(panels) {
		return ErrUnsupported
	}
	labels := make(map[string]string, len(tabs))
	for _, tab := range tabs {
		id := attribute(tab, "id")
		if id == "" || strings.ContainsFunc(id, htmlSpace) {
			return ErrUnsupported
		}
		if _, duplicate := labels[id]; duplicate {
			return ErrUnsupported
		}
		label, err := w.tabLabel(tab)
		if err != nil {
			return err
		}
		labels[id] = label
	}
	if err := w.uniqueIDs(root, labels); err != nil {
		return err
	}
	w.panelLabels = make(map[*html.Node]string, len(panels))
	for _, panel := range panels {
		id := attribute(panel, "aria-labelledby")
		label, ok := labels[id]
		if !ok {
			return ErrUnsupported
		}
		delete(labels, id)
		w.panelLabels[panel] = label
	}
	return nil
}

// tabLabel folds a tab's inline text the same way as prose blocks.
func (w *extractor) tabLabel(tab *html.Node) (string, error) {
	var b strings.Builder
	space := false
	var visit func(*html.Node) error
	visit = func(n *html.Node) error {
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			switch child.Type {
			case html.CommentNode:
			case html.TextNode:
				for _, c := range child.Data {
					if unicode.IsSpace(c) {
						space = true
						continue
					}
					if space && b.Len() > 0 {
						b.WriteByte(' ')
					}
					space = false
					b.WriteRune(c)
					if b.Len() > w.limits.MaxBlockBytes {
						return ErrLimit
					}
				}
			case html.ElementNode:
				if child.Namespace != "" || !inlineTag(child.Data) {
					return ErrUnsupported
				}
				if w.omit(child) {
					continue
				}
				if err := visit(child); err != nil {
					return err
				}
			default:
				return ErrUnsupported
			}
		}
		return nil
	}
	if err := visit(tab); err != nil {
		return "", err
	}
	if b.Len() == 0 {
		return "", ErrUnsupported
	}
	return b.String(), nil
}

// uniqueIDs requires each tab ID to occur exactly once in the whole document,
// because aria-labelledby resolves against the document, not the article.
func (w *extractor) uniqueIDs(root *html.Node, ids map[string]string) error {
	seen := make(map[string]bool, len(ids))
	var visit func(*html.Node) error
	visit = func(n *html.Node) error {
		if err := w.ctx.Err(); err != nil {
			return err
		}
		if n.Type == html.ElementNode {
			if id := attribute(n, "id"); id != "" {
				if _, tracked := ids[id]; tracked {
					if seen[id] {
						return ErrUnsupported
					}
					seen[id] = true
				}
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			if err := visit(child); err != nil {
				return err
			}
		}
		return nil
	}
	return visit(root)
}

// hasTabButton reports whether the walk would reach a <button role="tab"> in
// the selected article's children.
func (w *extractor) hasTabButton(selected *html.Node) (bool, error) {
	var visit func(*html.Node) (bool, error)
	visit = func(n *html.Node) (bool, error) {
		if err := w.ctx.Err(); err != nil {
			return false, err
		}
		if n.Type != html.ElementNode || w.omit(n) || skipped(n.Data) {
			return false, nil
		}
		if n.Data == "button" && attribute(n, "role") == "tab" {
			return true, nil
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			if found, err := visit(child); err != nil || found {
				return found, err
			}
		}
		return false, nil
	}
	for child := selected.FirstChild; child != nil; child = child.NextSibling {
		if found, err := visit(child); err != nil || found {
			return found, err
		}
	}
	return false, nil
}

// skipped matches the elements whose content the walk ignores.
func skipped(tag string) bool {
	switch tag {
	case "script", "style", "template", "meta", "link":
		return true
	}
	return false
}
