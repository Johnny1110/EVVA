package web

import (
	"io"
	"regexp"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// skipTags identifies subtrees whose text adds noise rather than signal —
// scripts, styles, embedded SVG, and chrome elements (nav/footer/aside).
// The model rarely wants any of this; suppressing it cuts a huge slice off
// every typical doc page and keeps the result inside FetchMaxBytes.
var skipTags = map[atom.Atom]bool{
	atom.Script:   true,
	atom.Style:    true,
	atom.Noscript: true,
	atom.Svg:      true,
	atom.Nav:      true,
	atom.Footer:   true,
	atom.Aside:    true,
	atom.Form:     true,
	atom.Iframe:   true,
}

// blockTags get a blank line break on entry/exit so paragraph structure
// survives the tree-walk. Inline tags (a, span, em, strong) flow inline.
var blockTags = map[atom.Atom]bool{
	atom.P:          true,
	atom.Div:        true,
	atom.Section:    true,
	atom.Article:    true,
	atom.Li:         true,
	atom.Ul:         true,
	atom.Ol:         true,
	atom.Tr:         true,
	atom.Td:         true,
	atom.Th:         true,
	atom.Pre:        true,
	atom.Blockquote: true,
	atom.H1:         true,
	atom.H2:         true,
	atom.H3:         true,
	atom.H4:         true,
	atom.H5:         true,
	atom.H6:         true,
	atom.Header:     true,
	atom.Main:       true,
}

// blankRunRE collapses 3+ newlines down to exactly 2 — preserves paragraph
// breaks while stopping a page full of empty <div>s from producing acres
// of vertical space in the model's context.
var blankRunRE = regexp.MustCompile(`\n{3,}`)

// extractReadableText parses HTML and returns a plain-text rendering with
// scripts/styles/nav stripped, anchors rendered as "text (href)", and
// block structure preserved as blank-line-separated paragraphs.
//
// Best-effort — html.Parse never returns an error for malformed input, so
// the only error path is reader I/O failure during the initial slurp.
func extractReadableText(body io.Reader) (string, error) {
	doc, err := html.Parse(body)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	walk(doc, &b)

	out := blankRunRE.ReplaceAllString(b.String(), "\n\n")
	return strings.TrimSpace(out), nil
}

func walk(n *html.Node, b *strings.Builder) {
	switch n.Type {
	case html.TextNode:
		// Preserve a single leading/trailing space — they matter for
		// inline runs like "<em>hi</em>, world". Empty/whitespace-only
		// nodes still need a space so adjacent inline tags don't collide.
		text := n.Data
		if strings.TrimSpace(text) == "" {
			if strings.ContainsAny(text, " \t\n") {
				b.WriteByte(' ')
			}
			return
		}
		b.WriteString(text)
		return

	case html.ElementNode:
		if skipTags[n.DataAtom] {
			return
		}
		// Anchor: render as "text (href)" so the model can chase links.
		if n.DataAtom == atom.A {
			href := attrValue(n, "href")
			start := b.Len()
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				walk(c, b)
			}
			if href != "" && b.Len() > start {
				b.WriteString(" (")
				b.WriteString(href)
				b.WriteByte(')')
			}
			return
		}
		// <br> → single newline.
		if n.DataAtom == atom.Br {
			b.WriteByte('\n')
			return
		}
		// Block element: surround children with blank-line gaps.
		if blockTags[n.DataAtom] {
			b.WriteString("\n\n")
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c, b)
		}
		if blockTags[n.DataAtom] {
			b.WriteString("\n\n")
		}
		return
	}

	// DocumentNode and others: just descend.
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walk(c, b)
	}
}

func attrValue(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}
