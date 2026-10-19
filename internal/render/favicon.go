package render

import (
	"strings"

	"golang.org/x/net/html"
)

// injectFavicon replaces stored icon links without reserializing the artifact.
// Tokenizing preserves scripts and handles head attributes and mixed case. The
// generated icon is last in the head so it takes precedence over stored icons.
func injectFavicon(body, uri string) string {
	link := `<link rel="icon" type="image/svg+xml" href="` + uri + `">`
	var out strings.Builder
	tokens := html.NewTokenizer(strings.NewReader(body))
	inserted := false
	for {
		kind := tokens.Next()
		if kind == html.ErrorToken {
			out.Write(tokens.Raw())
			break
		}
		raw := string(tokens.Raw())
		if kind == html.StartTagToken || kind == html.SelfClosingTagToken {
			token := tokens.Token()
			if token.Data == "link" {
				icon := false
				for _, attr := range token.Attr {
					if attr.Key == "rel" {
						for _, rel := range strings.Fields(strings.ToLower(attr.Val)) {
							icon = icon || rel == "icon"
						}
					}
				}
				if icon {
					continue
				}
			}
		}
		if kind == html.EndTagToken && tokens.Token().Data == "head" && !inserted {
			out.WriteString(link)
			inserted = true
		}
		out.WriteString(raw)
	}
	if !inserted {
		return link + out.String()
	}
	return out.String()
}
