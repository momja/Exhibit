package agent

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/momja/Exhibit/internal/store"
)

// A stored conversation is Pi's session file, and that is for Pi: it holds the
// system prompt, every tool result in full, the model's reasoning, and the
// provider and model that answered. None of that is what a person looking back
// at a conversation needs, and some of it is not theirs to be shown (on a
// platform instance the model is deliberately unreported, redact.go).
//
// So what leaves the server is a projection: the things a person saw — what
// they said, what the assistant said, which tools it used — built from named
// fields, so a field Pi adds later is not published by default.

// TranscriptMessage is one thing a person saw in a conversation.
type TranscriptMessage struct {
	// Role is "user", "assistant", or "tool" for a tool the assistant used.
	Role string `json:"role"`
	// Text is what was said; for a tool, what it did ("Editing artifact").
	Text string `json:"text"`
	// Images counts the pictures attached to something the user said. They are
	// not shown: a session file carries them inline as base64.
	Images int `json:"images,omitempty"`
	// Failed marks a tool that returned an error.
	Failed bool `json:"failed,omitempty"`
}

// titleRunes is the longest a conversation's title gets.
const titleRunes = 80

// ConversationTitle shortens a conversation's first prompt to a line a list can
// show.
func ConversationTitle(prompt string) string {
	line := strings.Join(strings.Fields(prompt), " ")
	if r := []rune(line); len(r) > titleRunes {
		return string(r[:titleRunes-1]) + "…"
	}
	return line
}

// piMessage is the part of Pi's message envelope the projection reads.
type piMessage struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content"`
	ToolCallID string          `json:"toolCallId"`
	IsError    bool            `json:"isError"`
}

// piBlock is one content block of a message.
type piBlock struct {
	Type      string         `json:"type"`
	Text      string         `json:"text"`
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// Messages returns what a person saw in a stored conversation, whichever form
// it was kept in.
func Messages(t *store.Transcript) []TranscriptMessage {
	if t.Resumable {
		return ProjectSessionFile(t.SessionFile)
	}
	return ProjectMessageDump(t.Messages)
}

// ProjectSessionFile returns what a person saw in a conversation kept as a Pi
// session file, in order. A line that is not an entry — anything unparseable —
// is skipped: the file is Pi's, and the projection reads what it understands.
func ProjectSessionFile(file string) []TranscriptMessage {
	var msgs []piMessage
	for _, line := range bytes.Split([]byte(file), []byte("\n")) {
		var entry struct {
			Type    string    `json:"type"`
			Message piMessage `json:"message"`
		}
		if json.Unmarshal(line, &entry) != nil || entry.Type != "message" {
			continue
		}
		msgs = append(msgs, entry.Message)
	}
	return project(msgs)
}

// ProjectMessageDump is ProjectSessionFile for a conversation kept before
// session files were: Pi's message list as one JSON array.
func ProjectMessageDump(dump string) []TranscriptMessage {
	var msgs []piMessage
	if json.Unmarshal([]byte(dump), &msgs) != nil {
		return nil
	}
	return project(msgs)
}

func project(msgs []piMessage) []TranscriptMessage {
	// A tool's outcome is a message of its own, after the call that asked for
	// it, so which calls failed is known up front.
	failed := map[string]bool{}
	for _, m := range msgs {
		if m.Role == "toolResult" && m.IsError {
			failed[m.ToolCallID] = true
		}
	}

	out := []TranscriptMessage{}
	for _, m := range msgs {
		switch m.Role {
		case "user":
			text, images := userContent(m.Content)
			out = append(out, TranscriptMessage{Role: "user", Text: text, Images: images})
		case "assistant":
			var blocks []piBlock
			if json.Unmarshal(m.Content, &blocks) != nil {
				continue
			}
			for _, b := range blocks {
				switch b.Type {
				case "text":
					if strings.TrimSpace(b.Text) != "" {
						out = append(out, TranscriptMessage{Role: "assistant", Text: b.Text})
					}
				case "toolCall":
					out = append(out, TranscriptMessage{Role: "tool", Text: toolLabel(b.Name, b.Arguments), Failed: failed[b.ID]})
				}
			}
		}
	}
	return out
}

// userContent flattens a user message — a string, or text and image blocks —
// to its text and a count of its images.
func userContent(raw json.RawMessage) (string, int) {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s, 0
	}
	var blocks []piBlock
	if json.Unmarshal(raw, &blocks) != nil {
		return "", 0
	}
	var text []string
	images := 0
	for _, b := range blocks {
		switch b.Type {
		case "text":
			text = append(text, b.Text)
		case "image":
			images++
		}
	}
	return strings.Join(text, "\n"), images
}

// toolLabel says what a tool call did, in the words the live chat uses for it
// (web/gallery/agent.js toolLabel). Arguments are read for a title or a key and
// nothing else: the rest of a call — an artifact's whole source, say — is not
// something to publish to read a conversation.
func toolLabel(name string, args map[string]any) string {
	str := func(k, fallback string) string {
		if v, ok := args[k].(string); ok && v != "" {
			return v
		}
		return fallback
	}
	switch name {
	case "create_artifact":
		return `Creating "` + str("title", "artifact") + `"`
	case "write_artifact":
		return "Writing artifact"
	case "edit_artifact":
		return "Editing artifact"
	case "get_artifact":
		return "Reading artifact source"
	case "set_widget":
		return "Saving widget"
	case "edit_widget":
		return "Editing widget"
	case "get_widget":
		return "Reading widget"
	case "get_state":
		return "Reading artifact state"
	case "set_state":
		return `Setting state key "` + str("key", "") + `"`
	case "delete_state":
		if k := str("key", ""); k != "" {
			return `Deleting state key "` + k + `"`
		}
		return "Erasing all state"
	case "get_selection":
		return "Reading selected elements"
	}
	return name
}
