package export

import (
	"strings"

	"golang.org/x/net/html"

	"github.com/momja/Exhibit/internal/inference/standin"
)

// DisableAI installs the unavailable window.exhibit.ai stand-in (Exh-i902) as
// the first thing inside the document's <head>, ahead of every artifact
// script, so an exported artifact that asks for AI is told no rather than
// throwing on undefined.
//
// Exports never carry AI, even when they are opened online: there is no
// endpoint, credential or instance configuration in an exported file to reach
// one with, and Exhibit does not rewire an artifact's calls. Nothing else in
// the document changes, so the artifact's own AI call sites stay in its
// source for whoever wants to wire up their own integration.
//
// Call it last, after every other head injection, because it must run first.
func DisableAI(doc string) string {
	at := headContentStart(doc)
	return doc[:at] + standin.Tag + doc[at:]
}

// headContentStart returns where the first element of the head goes: just past
// the <head> start tag when the document has one, or else just past the
// doctype and an <html> start tag, where the parser opens an implicit head.
// Never before the doctype, which would drop the document into quirks mode.
//
// It tokenizes rather than searching for "<head" so that a comment, a <header>
// or a string holding markup is not mistaken for the element, and sums raw
// token lengths so the document is returned byte-for-byte otherwise.
func headContentStart(doc string) int {
	z := html.NewTokenizer(strings.NewReader(doc))
	insert := 0
	for at := 0; ; {
		tt := z.Next()
		if tt == html.ErrorToken {
			return insert
		}
		// Raw first: TagName may reuse the buffer it returns.
		raw := len(z.Raw())
		switch tt {
		case html.DoctypeToken:
			insert = at + raw
		case html.CommentToken:
			// Comments may sit before or after the doctype; skip them.
		case html.TextToken:
			if strings.TrimSpace(string(z.Raw())) != "" {
				return insert
			}
		case html.StartTagToken:
			switch name, _ := z.TagName(); string(name) {
			case "html":
				insert = at + raw
			case "head":
				return at + raw
			default:
				return insert
			}
		default:
			return insert
		}
		at += raw
	}
}
