// Package scanner provides static analysis of HTML artifact source to extract
// outbound network origins. This is transparency, not enforcement — the CSP
// is the actual enforcement boundary. The scanner produces a deduplicated
// list of origins the artifact will likely contact so the user can approve
// them before the artifact is rendered.
package scanner

import (
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// Scan parses an HTML document and returns a deduplicated list of origins
// referenced by src, href, action attributes and inline fetch/import calls.
func Scan(body string) []string {
	seen := make(map[string]struct{})
	add := func(raw string) {
		origin := extractOrigin(raw)
		if origin != "" {
			seen[origin] = struct{}{}
		}
	}

	// Parse the HTML document
	doc, err := html.Parse(strings.NewReader(body))
	if err == nil {
		walkHTML(doc, add)
	}

	// Heuristic pass over the raw source for fetch/import literals
	for _, m := range fetchPattern.FindAllStringSubmatch(body, -1) {
		if len(m) > 1 {
			add(m[1])
		}
	}
	for _, m := range importPattern.FindAllStringSubmatch(body, -1) {
		if len(m) > 1 {
			add(m[1])
		}
	}

	origins := make([]string, 0, len(seen))
	for o := range seen {
		origins = append(origins, o)
	}
	return origins
}

// walkHTML traverses the HTML node tree and calls add for each URL attribute found.
func walkHTML(n *html.Node, add func(string)) {
	if n.Type == html.ElementNode {
		for _, attr := range n.Attr {
			switch {
			case attr.Key == "src" || attr.Key == "action":
				add(attr.Val)
			case attr.Key == "href" && n.Data != "a":
				// Include stylesheet/script links but not navigation links
				add(attr.Val)
			case n.Data == "a" && attr.Key == "href":
				// Skip anchor hrefs — they're navigation, not network fetch targets
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walkHTML(c, add)
	}
}

// fetchPattern matches URL string literals in fetch() and XMLHttpRequest calls.
var fetchPattern = regexp.MustCompile(`fetch\(\s*['"]([^'"]+)['"]`)

// importPattern matches ESM import URL literals.
var importPattern = regexp.MustCompile(`(?:import|from)\s+['"]([^'"]+)['"]`)

// extractOrigin returns the scheme+host origin from a URL string,
// or empty string if the URL is relative or invalid.
func extractOrigin(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.HasPrefix(raw, "//") {
		if strings.HasPrefix(raw, "//") {
			raw = "https:" + raw
		} else {
			return ""
		}
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if u.Host == "" {
		return "" // relative URL
	}
	scheme := u.Scheme
	if scheme == "" {
		scheme = "https"
	}
	if scheme != "http" && scheme != "https" {
		return "" // skip data:, blob:, etc.
	}
	return scheme + "://" + u.Host
}
