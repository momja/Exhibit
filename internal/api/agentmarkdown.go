package api

import (
	"bytes"
	"encoding/json"
	"html/template"
	"io"
	"strings"
	"time"

	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/extension"
	"github.com/yuin/goldmark/v2/parser"
	"github.com/yuin/goldmark/v2/renderer"
	"github.com/yuin/goldmark/v2/renderer/html"
)

var agentMarkdownParser = parser.New(parser.WithExtensions(extension.GFMParser))
var agentMarkdownRenderer = html.New(
	html.WithExtensions(extension.GFMHTMLRenderer),
	html.WithNodeRendererDecorator(ast.KindLink, inertUnsafeLinks),
	html.WithNodeRendererDecorator(ast.KindAutoLink, inertUnsafeLinks),
	// Images would fetch their URLs without a click. Render their alt-text
	// children with Goldmark's standard renderers, without an image element.
	html.WithNodeRendererDecorator(ast.KindImage, func(_ html.NodeRenderer) html.NodeRenderer {
		return html.NodeRendererFunc(func(_ io.Writer, _ []byte, _ ast.Node, _ bool, _ renderer.Context) (ast.WalkStatus, error) {
			return ast.WalkContinue, nil
		})
	}),
)

// Goldmark normally gives blocked links an empty href, which reloads the chat
// when clicked. Reuse its URL classification, rendering just the label instead.
func inertUnsafeLinks(next html.NodeRenderer) html.NodeRenderer {
	return html.NodeRendererFunc(func(w io.Writer, source []byte, node ast.Node, entering bool, ctx renderer.Context) (ast.WalkStatus, error) {
		switch link := node.(type) {
		case *ast.Link:
			if html.IsDangerousURL(link.Destination.Value(source)) {
				return ast.WalkContinue, nil
			}
		case *ast.AutoLink:
			if html.IsDangerousURL(link.Destination.Value(source)) {
				if entering {
					_, err := link.Label.WriteTo(html.ContextTextWriter(ctx), source)
					return ast.WalkContinue, err
				}
				return ast.WalkContinue, nil
			}
		}
		return next.Render(w, source, node, entering, ctx)
	})
}

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
const agentMarkdownInterval = 100 * time.Millisecond

type agentMarkdownStream struct {
	text         strings.Builder
	contentIndex int
	oversized    bool
	rendered     bool
	dirty        bool
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
	if event.Type == "agent_end" || event.Type == "agent_settled" || event.Type == "exhibit_session_closed" {
		if stream.dirty {
			line = stream.renderSnapshot(line)
		}
		*stream = agentMarkdownStream{}
		return line
	}
	if event.Type == "message_start" {
		*stream = agentMarkdownStream{}
	}
	if event.Type != "message_update" {
		return line
	}
	update := event.Update
	if update.Type == "text_start" || update.ContentIndex != stream.contentIndex {
		*stream = agentMarkdownStream{contentIndex: update.ContentIndex}
	}
	var source string
	switch update.Type {
	case "text_delta":
		if stream.oversized {
			return line
		}
		if stream.text.Len()+len(update.Delta) > maxAgentMarkdownBytes {
			source := stream.text.String() + update.Delta
			stream.text.Reset()
			stream.oversized = true
			stream.dirty = false
			return decorateAgentEvent(line, "exhibitMessageText", source)
		}
		stream.text.WriteString(update.Delta)
		stream.dirty = true
		if stream.rendered {
			return decorateAgentEvent(line, "exhibitMessagePending", true)
		}
		return stream.renderSnapshot(line)
	case "text_end":
		source = update.Content
		*stream = agentMarkdownStream{}
		if len(source) > maxAgentMarkdownBytes {
			return line
		}
	default:
		return line
	}
	return renderAgentEvent(line, source)
}

// flush emits at most one snapshot per interval even during rapid token bursts
// or replay. Raw Pi events still reach API consumers unchanged apart from the
// pending marker. Completion and interruption always flush immediately.
func (stream *agentMarkdownStream) flush() []byte {
	if !stream.dirty {
		return nil
	}
	return stream.renderSnapshot([]byte(`{"type":"exhibit_message"}`))
}

func (stream *agentMarkdownStream) renderSnapshot(line []byte) []byte {
	stream.rendered = true
	stream.dirty = false
	return renderAgentEvent(line, stream.text.String())
}

func renderAgentEvent(line []byte, source string) []byte {
	fragment, err := renderPage("agentMessageBody", source)
	if err != nil {
		return line
	}
	return decorateAgentEvent(line, "exhibitMessageHTML", fragment)
}

func decorateAgentEvent(line []byte, key string, value any) []byte {
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(line, &fields)
	fields[key], _ = json.Marshal(value)
	rendered, _ := json.Marshal(fields)
	return rendered
}
