// Package scanner provides static analysis of HTML artifact source to extract
// outbound network origins. This is transparency, not enforcement — the CSP
// is the actual enforcement boundary. The scanner produces a deduplicated
// list of origins the artifact will likely contact so the user can approve
// them before the artifact is rendered.
package scanner

import (
	"log/slog"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// Scan parses an HTML document and returns a deduplicated list of origins
// referenced by src, href, action attributes and inline fetch/import calls.
// Relative references are dropped — use ScanWithBase to resolve them.
func Scan(body string) []string {
	return scan(body, nil)
}

// ScanWithBase behaves like Scan but, when baseURL is a non-empty absolute
// http(s) URL, resolves relative references against it so residual external
// origins still surface in the footprint. This matters for snapshot imports:
// anything that couldn't be inlined (runtime-constructed fetch URLs, over-limit
// assets) would otherwise be silently dropped and 404 at render time. When
// baseURL is empty or not an absolute http(s) URL, the result equals Scan(body).
// Absolute references are unaffected by the base in either entry point.
func ScanWithBase(body, baseURL string) []string {
	return scan(body, parseBase(baseURL))
}

// scan is the shared implementation behind Scan and ScanWithBase. A nil base
// drops relative references; a non-nil base resolves them to their real origin.
func scan(body string, base *url.URL) []string {
	seen := make(map[string]struct{})
	add := func(raw string) {
		origin := resolveOrigin(raw, base)
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
	slog.Debug("scan complete", slog.Int("origins", len(origins)), slog.Any("footprint", origins))
	return origins
}

// parseBase returns the parsed base URL when baseURL is a non-empty absolute
// http(s) URL, and nil otherwise. A nil base makes ScanWithBase equal Scan.
func parseBase(baseURL string) *url.URL {
	if baseURL == "" {
		return nil
	}
	u, err := url.Parse(baseURL)
	if err != nil || !u.IsAbs() || u.Host == "" {
		return nil
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil
	}
	return u
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

// resolveOrigin returns the scheme+host origin from a URL string. Absolute and
// protocol-relative references are reduced to their origin directly (base has no
// effect on them). A path-relative reference (no host) is resolved against base
// when one is supplied; without a base it is dropped, exactly as before. Note a
// relative that resolves to the base's own origin is still reported: from the
// artifact's opaque-origin sandbox that origin is external.
func resolveOrigin(raw string, base *url.URL) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "//") {
		raw = "https:" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if u.Host == "" {
		// A path-relative reference (data:/blob:/mailto: are absolute — IsAbs
		// is true — so they fall through and are dropped by the scheme check).
		if base == nil || u.IsAbs() {
			return ""
		}
		u = base.ResolveReference(u)
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
