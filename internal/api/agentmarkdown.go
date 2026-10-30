package api

import (
	"bytes"
	"encoding/json"
	"html/template"
	"strings"

	"github.com/yuin/goldmark/v2/extension"
	"github.com/yuin/goldmark/v2/parser"
	"github.com/yuin/goldmark/v2/renderer/html"
)

var agentMarkdownParser = parser.New(parser.WithExtensions(extension.GFMParser))
var agentMarkdownRenderer = html.New(html.WithExtensions(extension.GFMHTMLRenderer))

// agentMarkdown is the only trusted-HTML boundary for conversation text. Keep
// Goldmark's safe defaults: raw HTML is omitted and dangerous URLs are blocked.
// Attributes and automatic heading IDs are deliberately not enabled.
func agentMarkdown(source string) (template.HTML, error) {
	text := []byte(source)
	var out bytes.Buffer
	if err := agentMarkdownRenderer.Render(&out, text, agentMarkdownParser.Parse(text)); err != nil {
		return "", err
	}
	return template.HTML(out.String()), nil
}

// Keep per-connection reconstruction bounded even if a provider never ends a
// text block. Oversized blocks use the browser's escaped plain-text fallback.
const maxAgentMarkdownBytes = 1 << 20

type agentMarkdownStream struct {
	text         strings.Builder
	contentIndex int
	oversized    bool
}

// renderEvent decorates Pi's delta-only wire events with the same partial used
// by history. text_end is authoritative, repairing even an incomplete replay.
// The session's retained events and stored transcript are never modified.
func (stream *agentMarkdownStream) renderEvent(line []byte) []byte {
	var event struct {
		Type   string `json:"type"`
		Update struct {
			Type         string `json:"type"`
			ContentIndex int    `json:"contentIndex"`
			Delta        string `json:"delta"`
			Content      string `json:"content"`
		} `json:"assistantMessageEvent"`
	}
	if json.Unmarshal(line, &event) != nil {
		return line
	}
	if event.Type == "message_start" || event.Type == "agent_settled" {
		stream.text.Reset()
		stream.oversized = false
	}
	if event.Type != "message_update" {
		return line
	}
	update := event.Update
	if update.Type == "text_start" || update.ContentIndex != stream.contentIndex {
		stream.text.Reset()
		stream.oversized = false
		stream.contentIndex = update.ContentIndex
	}
	var source string
	switch update.Type {
	case "text_delta":
		if stream.oversized {
			return line
		}
		if stream.text.Len()+len(update.Delta) > maxAgentMarkdownBytes {
			stream.text.Reset()
			stream.oversized = true
			return line
		}
		stream.text.WriteString(update.Delta)
		source = stream.text.String()
	case "text_end":
		source = update.Content
		stream.text.Reset()
		stream.oversized = false
		if len(source) > maxAgentMarkdownBytes {
			return line
		}
	default:
		return line
	}
	fragment, err := renderPage("agentMessageBody", source)
	if err != nil {
		return line
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(line, &fields)
	fields["exhibitMessageHTML"], _ = json.Marshal(fragment)
	rendered, _ := json.Marshal(fields)
	return rendered
}
