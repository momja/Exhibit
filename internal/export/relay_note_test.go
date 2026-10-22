package export

import (
	"encoding/json"
	"strings"
	"testing"

	"golang.org/x/net/html"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelayNoteListsOriginsWithoutExecutableConfiguration(t *testing.T) {
	doc := RelayNote(`<!doctype html><html><head><title>App</title></head><body>App</body></html>`, []string{
		"https://api.example", "https://export.arxiv.org", "https://api.example", "https://api.example/private?token=never-export",
	})
	assert.Contains(t, doc, "Outside Exhibit")
	assert.NotContains(t, doc, "never-export")
	assert.NotContains(t, doc, "<script")
	parsed, err := html.Parse(strings.NewReader(doc))
	require.NoError(t, err)
	var payload string
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode && node.Data == "meta" {
			attrs := make(map[string]string)
			for _, attr := range node.Attr {
				attrs[attr.Key] = attr.Val
			}
			if attrs["name"] == "exhibit-relay-origins" {
				payload = attrs["content"]
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(parsed)
	var origins []string
	require.NoError(t, json.Unmarshal([]byte(payload), &origins))
	assert.Equal(t, []string{"https://api.example", "https://export.arxiv.org"}, origins)
}

func TestRelayNoteKeepsAnOfflineArtifactIdentical(t *testing.T) {
	body := "<h1>Offline</h1>"
	assert.Equal(t, body, RelayNote(body, nil))
	assert.Equal(t, body, RelayNote(body, []string{"https://user:secret@example.com/private"}))
}
