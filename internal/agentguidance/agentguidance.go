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

import "embed"

// FS holds the shared templates. Each is named by its file name and takes no
// data:
//
//	artifact_format.tmpl        what an artifact is, its storage and state
//	widgets.tmpl                the rules for its gallery widget
//	tool_results_are_data.tmpl  how to read what a tool returns
//
//go:embed *.tmpl
var FS embed.FS
