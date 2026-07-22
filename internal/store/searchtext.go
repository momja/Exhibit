package store

import (
	"strings"

	"golang.org/x/net/html"
)

// invisibleElements are elements whose contents are never rendered as page
// text — indexing them would make gallery search match markup and code every
// artifact shares (every artifact has a <script>, so "script" would match
// everything), which is worse than not indexing them at all.
var invisibleElements = map[string]bool{
	"script":   true,
	"style":    true,
	"noscript": true,
	"template": true,
}

// semanticAttributes carry human-meaningful text that isn't a text node but
// describes what the artifact shows; worth searching over.
var semanticAttributes = []string{"alt", "title", "placeholder", "aria-label"}

// ExtractSearchText reduces an artifact's HTML source to the text a user
// would actually see if they opened it: rendered text nodes plus a few
// semantic attribute values (alt/title/placeholder/aria-label...). Script,
// style, and markup are dropped, so gallery search matches what an artifact
// *is* rather than the code it's made of (av-b6o9). The result feeds only
// the artifacts_fts index — the blob store remains the source of truth for
// the body itself.
func ExtractSearchText(source string) string {
	doc, err := html.Parse(strings.NewReader(source))
	if err != nil {
		// html.Parse is lenient and essentially never fails on string input;
		// if it somehow does, indexing the raw source beats indexing nothing.
		return source
	}

	var b strings.Builder
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && invisibleElements[n.Data] {
			return
		}
		if n.Type == html.ElementNode {
			for _, attr := range semanticAttributes {
				if v := getAttr(n, attr); v != "" {
					b.WriteString(v)
					b.WriteByte(' ')
				}
			}
		}
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
			b.WriteByte(' ')
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	// Collapse whitespace runs so the index stores clean prose, not
	// indentation artifacts from pretty-printed HTML.
	return strings.Join(strings.Fields(b.String()), " ")
}

func getAttr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}
