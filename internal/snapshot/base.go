package snapshot

import (
	"golang.org/x/net/html"
)

// InjectBaseHref inserts <base href="pageURL"> at the top of the document's
// <head>, so relative references that survive ingest — snapshot disabled, or
// residual after a partial snapshot — resolve against the source site instead
// of the render origin (option A of the relative-URL design, exhibit-lwb).
// Whether the resolved origin is then reachable stays the CSP allowlist's
// decision; this only fixes what the reference points at.
//
// A document that already declares its own <base> keeps it untouched: the
// author's base governs resolution, and browsers ignore every <base> after
// the first anyway. Insertion mirrors the render surface's injectShim string
// idiom (after <head>, else before </head>, else prepend — HTML tree
// construction moves a leading <base> into the implicit head).
func InjectBaseHref(body, pageURL string) string {
	if pageURL == "" || containsBaseTag(body) {
		return body
	}
	tag := `<base href="` + html.EscapeString(pageURL) + `">`
	if idx := indexASCIIFold(body, "<head>"); idx >= 0 {
		at := idx + len("<head>")
		return body[:at] + tag + body[at:]
	}
	if idx := indexASCIIFold(body, "</head>"); idx >= 0 {
		return body[:idx] + tag + body[idx:]
	}
	return tag + body
}

// containsBaseTag reports whether the document appears to declare a <base>
// element. A false positive (the token inside a comment or script) merely
// skips injection and leaves the document as-is — the safe failure mode.
func containsBaseTag(body string) bool {
	for i := 0; ; {
		j := indexASCIIFold(body[i:], "<base")
		if j < 0 {
			return false
		}
		i += j + len("<base")
		if i >= len(body) {
			return false
		}
		switch body[i] {
		case ' ', '\t', '\n', '\r', '\f', '/', '>':
			return true
		}
	}
}

// indexASCIIFold returns the index of the first ASCII-case-insensitive match
// of token (which must be lowercase ASCII) in s, or -1. Matching over the
// original bytes — instead of indexing into s with positions computed on
// strings.ToLower(s) — matters: Unicode lowering can change byte length
// (e.g. the 3-byte Kelvin sign 'K' lowers to a 1-byte 'k'), which would shift
// every later index and corrupt the insertion point.
func indexASCIIFold(s, token string) int {
outer:
	for i := 0; i+len(token) <= len(s); i++ {
		for j := 0; j < len(token); j++ {
			c := s[i+j]
			if 'A' <= c && c <= 'Z' {
				c += 'a' - 'A'
			}
			if c != token[j] {
				continue outer
			}
		}
		return i
	}
	return -1
}
