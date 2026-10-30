package api

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/html"
)

func TestAgentMarkdown(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		want         []string
	}{
		{"formatting", "# Heading\n\n**bold** and *emphasis*, ~~removed~~ and `code`.\n\n> quote\n\n- one\n  - nested\n\n1. first\n2. second\n\n---", []string{"<h1>Heading</h1>", "<strong>bold</strong>", "<em>emphasis</em>", "<del>removed</del>", "<code>code</code>", "<blockquote>", "<ul>", "<ol>", "<hr>"}},
		{"GFM", "- [x] ready\n- [ ] pending\n\n| Name | Value |\n| --- | --- |\n| table | cell |\n\nhttps://example.com", []string{`type="checkbox"`, "disabled", "checked", "<table>", "<td>cell</td>", `href="https://example.com"`}},
		{"unfinished fence", "```html\n<script>alert('x')</script>", []string{`<pre><code class="language-html">`, "&lt;script&gt;"}},
		{"unfinished emphasis", "A **part", []string{"<p>A **part</p>"}},
		{"entities", "&lt;img src=x onerror=alert(1)&gt; &amp;", []string{"&lt;img src=x onerror=alert(1)&gt; &amp;"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rendered, err := agentMarkdown(tc.source)
			require.NoError(t, err)
			for _, want := range tc.want {
				assert.Contains(t, string(rendered), want)
			}
		})
	}
}

func TestAgentMarkdownRejectsHTMLAndDangerousURLs(t *testing.T) {
	for _, source := range []string{
		`<script>window.pwned = true</script>`,
		`<style>body{display:none}</style>`,
		`<iframe srcdoc="<script>alert(1)</script>"></iframe>`,
		`<img src=x onerror=alert(1)>`,
		`<a href="javascript:alert(1)" onclick="alert(1)">raw</a>`,
		`<div hx-get="/api/artifacts" data-controller="evil">raw</div>`,
		"[bad](javascript:alert%281%29) [bad](javascript&colon;alert%281%29)",
		"[bad](java&#x73;cript:alert%281%29) [bad](vbscript:msgbox%281%29)",
		"![bad](data:text/html;base64,PHNjcmlwdD4=) ![bad](data:image/svg+xml;base64,PHN2Zz4=)",
		"[bad](javascript&#58;alert%281%29) [bad](data:text/html,hello)",
		"# heading {#input onclick=alert(1)}",
	} {
		t.Run(source, func(t *testing.T) {
			rendered, err := renderPage("agentMessage", source)
			require.NoError(t, err)
			doc, err := html.Parse(strings.NewReader(rendered))
			require.NoError(t, err)
			var visit func(*html.Node)
			visit = func(n *html.Node) {
				if n.Type == html.ElementNode {
					assert.NotContains(t, []string{"script", "style", "iframe", "object", "svg"}, n.Data)
					for _, a := range n.Attr {
						assert.False(t, strings.HasPrefix(a.Key, "on") || strings.HasPrefix(a.Key, "hx-") || a.Key == "data-controller", "unsafe attribute: %v", a)
						if a.Key == "href" || a.Key == "src" {
							assert.NotContains(t, strings.ToLower(a.Val), "javascript:")
							assert.NotContains(t, strings.ToLower(a.Val), "vbscript:")
							assert.NotContains(t, strings.ToLower(a.Val), "data:")
						}
					}
				}
				for child := n.FirstChild; child != nil; child = child.NextSibling {
					visit(child)
				}
			}
			visit(doc)
			assert.Contains(t, rendered, "data-message-source=")
		})
	}
}

func markdownWireEvent(kind, value string, index int) []byte {
	field := "delta"
	if kind == "text_end" {
		field = "content"
	}
	line, _ := json.Marshal(map[string]any{"type": "message_update", "assistantMessageEvent": map[string]any{"type": kind, "contentIndex": index, field: value}})
	return line
}

func TestAgentMarkdownStream(t *testing.T) {
	var stream agentMarkdownStream
	fragment := func(line []byte) string {
		t.Helper()
		var event struct {
			HTML string `json:"exhibitMessageHTML"`
		}
		require.NoError(t, json.Unmarshal(stream.renderEvent(line), &event))
		return event.HTML
	}
	assert.Contains(t, fragment(markdownWireEvent("text_delta", "**bo", 1)), "<p>**bo</p>")
	assert.Contains(t, fragment(markdownWireEvent("text_delta", "ld**", 1)), "<strong>bold</strong>")
	assert.Contains(t, fragment(markdownWireEvent("text_end", "**authoritative**", 1)), "<strong>authoritative</strong>")
	assert.Contains(t, fragment(markdownWireEvent("text_delta", "new block", 3)), "<p>new block</p>")
	stream.renderEvent([]byte(`{"type":"message_start"}`))
	assert.Contains(t, fragment(markdownWireEvent("text_delta", "new message", 3)), "<p>new message</p>")
	// A reconnect may begin in the middle of a retained block; text_end repairs it.
	stream = agentMarkdownStream{}
	fragment(markdownWireEvent("text_delta", "ld**", 1))
	assert.Contains(t, fragment(markdownWireEvent("text_end", "**bold**", 1)), "<strong>bold</strong>")
	assert.Empty(t, fragment(markdownWireEvent("text_delta", strings.Repeat("x", maxAgentMarkdownBytes+1), 2)))
	assert.Zero(t, stream.text.Len())
	assert.Empty(t, fragment(markdownWireEvent("text_delta", "more", 2)))
	assert.Contains(t, fragment(markdownWireEvent("text_end", "recovered", 2)), "<p>recovered</p>")
	for _, line := range []string{`not JSON`, `{"type":"agent_start"}`, `{"type":"message_update","assistantMessageEvent":{"type":"thinking_delta","delta":"private"}}`} {
		assert.Equal(t, line, string(stream.renderEvent([]byte(line))))
	}
}

func TestAgentMarkdownConcurrentRendering(t *testing.T) {
	// History requests and SSE clients share the configured parser and renderer.
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Go(func() {
			for j := 0; j < 20; j++ {
				source := fmt.Sprintf("**message %d**", j)
				rendered, err := agentMarkdown(source)
				assert.NoError(t, err)
				assert.Contains(t, string(rendered), fmt.Sprintf("<strong>message %d</strong>", j))
			}
		})
	}
	wg.Wait()
}
