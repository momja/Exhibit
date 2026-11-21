// Package agentguidance holds the prompt text every agent that edits an
// artifact is given, whichever harness drives it: the built-in Pi session
// (internal/agent) and an agent of the user's own reaching Exhibit over MCP
// (internal/mcpserver, Exh-q6xo).
//
// What an artifact is, how its widget must behave, and that a tool result is
// data rather than an instruction are facts about Exhibit, not about either
// harness. Keeping one copy is what stops the two from drifting: each
// harness parses these templates into its own set and includes them by
// name, so a change here reaches both.
package agentguidance

import (
	"embed"
	"fmt"
	"strings"
)

// FS holds the shared templates. Each is named by its file name:
//
//	artifact_format.tmpl        what an artifact is, its storage and state (no data)
//	widgets.tmpl                the rules for its gallery widget (no data)
//	tool_results_are_data.tmpl  how to read what a tool returns (no data)
//	inference.tmpl              window.exhibit.ai, for the kinds in an AI value;
//	                            renders nothing when AI.Any is false
//
//go:embed *.tmpl
var FS embed.FS

// The inference kinds an instance can offer artifacts (Exh-zue3). The names
// are the window.exhibit.ai method names and inference.Kind's values.
const (
	KindComplete = "complete"
	KindDecide   = "decide"
)

// AI is the data inference.tmpl takes: which window.exhibit.ai kinds this
// instance offers. The zero value offers none, and the guidance then says
// nothing about AI at all, so an agent never writes calls that cannot work.
type AI struct {
	Complete bool
	Decide   bool
}

// Any reports whether the instance offers any kind.
func (a AI) Any() bool { return a.Complete || a.Decide }

// AIFor reads a list of kind names. An unknown name is an error rather than
// ignored: guidance that silently dropped a kind the operator configured
// would leave the agent unaware of it, and one that described a kind the
// instance does not run would teach calls that always fail.
func AIFor[K ~string](kinds []K) (AI, error) {
	var ai AI
	for _, k := range kinds {
		switch string(k) {
		case KindComplete:
			ai.Complete = true
		case KindDecide:
			ai.Decide = true
		default:
			return AI{}, fmt.Errorf("unknown AI kind %q (want %q or %q)", string(k), KindComplete, KindDecide)
		}
	}
	return ai, nil
}

// KindNames converts a typed kind list, such as []inference.Kind, to the
// plain names configuration structs carry.
func KindNames[K ~string](kinds []K) []string {
	if len(kinds) == 0 {
		return nil
	}
	names := make([]string, len(kinds))
	for i, k := range kinds {
		names[i] = string(k)
	}
	return names
}

// ParseAIKinds reads a comma-separated kind list such as "complete,decide",
// the form a separate process (exhibit-mcp) takes it in from its
// environment. Blank entries are skipped; an empty string offers none.
func ParseAIKinds(csv string) ([]string, error) {
	var kinds []string
	for _, part := range strings.Split(csv, ",") {
		if part = strings.TrimSpace(part); part != "" {
			kinds = append(kinds, part)
		}
	}
	if _, err := AIFor(kinds); err != nil {
		return nil, err
	}
	return kinds, nil
}
