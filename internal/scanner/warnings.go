package scanner

import (
	"net/url"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/net/html"
)

// Warning explains a likely networking mistake without treating the static
// scan as a security decision. Origin is present only when one was identified.
type Warning struct {
	Code    string `json:"code"`
	Origin  string `json:"origin,omitempty"`
	Message string `json:"message"`
}

var publicProxyHosts = []string{
	"allorigins.win", "cors-anywhere.herokuapp.com", "cors.eu.org",
	"cors.isomorphic-git.org", "corsproxy.github.io", "corsproxy.io",
	"corsproxy.org", "proxy.cors.sh", "thingproxy.freeboard.io",
}

var noCORSModePattern = regexp.MustCompile("(?i)(?:\\bmode|[\"']mode[\"'])\\s*:\\s*[\"'`]no-cors[\"'`]")

// Warnings reports public CORS proxies, unreadable no-cors responses, and a
// copied document's remaining calls to its original server. Results are stable
// and deduplicated, so source edits and the approval UI can share this report.
// Like ScanDoc, this is a heuristic; runtime approval remains authoritative.
func Warnings(body, sourceURL string) []Warning {
	warnings := make([]Warning, 0)
	lower := strings.ToLower(body)
	for _, host := range publicProxyHosts {
		// Also catch dynamically composed proxy URLs and bare host strings, not
		// only the URL literals visible to the origin scan. Bound the hostname
		// so a harmless corsproxy.io.example reference is not attributed to it.
		pattern := regexp.MustCompile(`(?:^|[^a-z0-9.-])(?:[a-z0-9-]+\.)*` + regexp.QuoteMeta(host) + `(?:$|[^a-z0-9.-])`)
		if pattern.MatchString(lower) {
			warnings = append(warnings, Warning{
				Code: "public_cors_proxy", Origin: "https://" + host,
				Message: "This public CORS proxy sends your requests through a third party; Exhibit doesn't need it when its relay is enabled. Fetch the API directly.",
			})
		}
	}
	if noCORSModePattern.MatchString(body) {
		warnings = append(warnings, Warning{
			Code:    "no_cors",
			Message: "mode: 'no-cors' creates an opaque, unreadable response and is deliberately not routed through Exhibit's relay. Remove it to read the API response.",
		})
	}

	sourceOrigin := resolveOrigin(sourceURL, nil)
	if sourceOrigin != "" {
		docBaseURL := sourceURL
		// The ingester adds a base to copied pages. Respect an explicit base
		// just as the browser does, then fall back to the known source URL.
		if doc, err := html.Parse(strings.NewReader(body)); err == nil {
			if base := docBase(doc); base != nil {
				docBaseURL = base.String()
			}
		}
		base := parseBase(docBaseURL)
		for _, raw := range FetchRefs(body) {
			if resolveOrigin(raw, base) == sourceOrigin {
				warnings = append(warnings, Warning{
					Code: "source_site_api", Origin: sourceOrigin,
					Message: "This copy still calls its original site's server. Approving these requests can make this saved copy a live client of the original site.",
				})
				break
			}
		}
	}
	return warnings
}

// FetchOrigins returns the origins of statically visible fetch calls. Asset
// references and navigation links do not need an API relay. Runtime-built URLs
// remain outside this heuristic and should be declared separately on export.
func FetchOrigins(body string) []string {
	doc, _ := html.Parse(strings.NewReader(body))
	var base *url.URL
	if doc != nil {
		base = docBase(doc)
	}
	seen := make(map[string]bool)
	for _, raw := range FetchRefs(body) {
		if origin := resolveOrigin(raw, base); origin != "" {
			seen[origin] = true
		}
	}
	origins := make([]string, 0, len(seen))
	for origin := range seen {
		origins = append(origins, origin)
	}
	sort.Strings(origins)
	return origins
}
