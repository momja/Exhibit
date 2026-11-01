package api

import (
	"fmt"
	"html"
	"net/url"
	"strings"
)

// A private widget still renders its saved data, but cannot navigate its
// opaque frame around the document's resource CSP. Its trusted app-origin
// wrapper has no script and pins that frame to this one widget path.
func widgetFramePolicy(widgetURL string) string {
	u, err := url.Parse(widgetURL)
	if err != nil || !strings.HasPrefix(u.Path, "/w/") {
		return "default-src 'none'; frame-src 'none'"
	}
	parts := strings.Split(strings.TrimPrefix(u.Path, "/w/"), "/")
	if len(parts) != 1 && !(len(parts) == 3 && parts[1] == "p" && len(parts[2]) == 64) {
		return "default-src 'none'; frame-src 'none'"
	}
	identifier := parts[0]
	if identifier == "" || identifier == "." || identifier == ".." || strings.Contains(identifier, "\\") {
		return "default-src 'none'; frame-src 'none'"
	}
	base := framePolicyOrigin(u.Scheme + "://" + u.Host)
	if base == "" || u.User != nil || u.Fragment != "" {
		return "default-src 'none'; frame-src 'none'"
	}
	return "default-src 'none'; style-src 'unsafe-inline'; frame-src " + base + u.EscapedPath() + "; base-uri 'none'; form-action 'none'"
}

func widgetFrameDocument(widgetURL string) string {
	return fmt.Sprintf(`<!doctype html><html><head><meta charset="utf-8"><meta http-equiv="Content-Security-Policy" content="%s"><meta name="referrer" content="origin"><style>html,body{margin:0;padding:0;height:100%%;overflow:hidden;background:transparent}iframe{display:block;border:0;width:100%%;height:100%%}</style></head><body><iframe src="%s" sandbox="allow-scripts" referrerpolicy="origin" title="Connected widget"></iframe></body></html>`, html.EscapeString(widgetFramePolicy(widgetURL)), html.EscapeString(widgetURL))
}

func (v widgetView) FrameDocument() string { return widgetFrameDocument(v.URL) }
