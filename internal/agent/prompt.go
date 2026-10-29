package agent

import (
	"bytes"
	"embed"
	"strings"
	"text/template"
)

// Every model-facing prompt lives in prompts/*.tmpl, one file per prompt, so
// the text can be read, reviewed and diffed as prose. They are parsed once at
// package init; a template that fails to parse stops the binary from starting
// rather than surfacing on the first session.
//
//go:embed prompts/*.tmpl
var promptFS embed.FS

var promptTemplates = template.Must(template.New("").Option("missingkey=error").ParseFS(promptFS, "prompts/*.tmpl"))

// renderPrompt executes prompts/<name>.tmpl. The single trailing newline an
// editor adds to the file is not part of the prompt. Templates take only
// server-authored values: nothing ingested from an artifact is ever passed in.
func renderPrompt(name string, data any) string {
	var out bytes.Buffer
	if err := promptTemplates.ExecuteTemplate(&out, name+".tmpl", data); err != nil {
		panic("agent: render prompt " + name + ": " + err.Error())
	}
	return strings.TrimSuffix(out.String(), "\n")
}

// GenerateWidgetPrompt is the entire user turn the "Generate widget" button
// sends. It is a server-side constant on purpose: the button takes no input, so
// there is nothing a caller can put into the model's context through that
// route. Everything that makes the result a *widget* is in the session's system
// prompt (CreateOpts.WidgetOnly), not here.
var GenerateWidgetPrompt = renderPrompt("generate_widget", nil)

// This file assembles a session's context: what Exhibit says (the system
// prompt), and how it tells the model to treat everything Exhibit did *not*
// author. That material — an artifact's source, its title, its stored state, an
// element the user picked — reaches the model only as tool results, never as
// text in a system or user message. A tool result is its own message role, so
// the boundary between data and instruction is the conversation's structure
// rather than a delimiter inside a string: there is nothing for content to
// forge, and nothing to carry from one process to the next when a chat is
// resumed. See docs/security.md §5.

// rolePrompt is the instruction half of the context, and it is entirely
// server-authored. No artifact title, body, id, or other ingested text is ever
// interpolated into it: a URL-ingested artifact's <title> comes from a remote
// page, and the system role is the last place attacker-authored text should
// sit (av-e0yj).
var rolePrompt = renderPrompt("role", nil)

// dataContract tells the model how to read what its tools return. It is
// appended to whatever role prompt is in force, so an operator override cannot
// drop it.
//
// It is static, and that is the point of the design: it names no per-session
// secret, because the boundary it describes is the conversation's own
// structure — tool results are a separate message role — rather than a marker
// inside a string. What it adds is the instruction the structure cannot carry:
// that a tool result is material, however much it reads like a command.
var dataContract = renderPrompt("data_contract", nil)

var questionGuidance = renderPrompt("question_guidance", nil)

// modePrompt is the paragraph naming what this particular session is for.
//
// The cases are mutually exclusive, which is why this is a switch and not two
// ifs. A widget-only session must NOT also get the edit-an-artifact paragraph:
// that one tells the model to save with edit_artifact/write_artifact, which is precisely
// what a "generate this artifact's tile" session must never do (av-fafu).
//
// Note what is absent: the artifact's id and title. The tools take no id, so
// naming one would be decoration — and the title is the single most
// attacker-controllable field on a URL-ingested artifact, so it reaches the
// model only inside a tool result or not at all (av-e0yj).
func modePrompt(opts CreateOpts) string {
	switch {
	case opts.WidgetOnly:
		return "\n\n" + renderPrompt("mode_widget_only", nil)
	case opts.ArtifactID != "":
		return "\n\n" + renderPrompt("mode_edit", nil)
	}
	return ""
}

// buildSystemPrompt composes the session's system prompt: the role half
// (overridable by config), the paragraph naming this session's mode, and the
// always-present contract on how to read tool results.
func buildSystemPrompt(override string, opts CreateOpts) string {
	role := override
	if role == "" {
		role = rolePrompt
	}
	guidance := ""
	if !opts.WidgetOnly {
		guidance = "\n\n" + questionGuidance
	}
	return role + modePrompt(opts) + guidance + "\n\n" + dataContract
}

// selectionNotice is the fixed sentence appended to a prompt that carries a
// selection. It states that elements were selected and where to read them; it
// carries none of their content, which is untrusted and arrives only through
// get_selection.
func selectionNotice(n int) string {
	return renderPrompt("selection_notice", struct{ Count int }{n})
}

// resumedNotice is the sentence the first prompt of a resumed conversation
// carries. Everything the model remembers of the artifact was true when the
// conversation last ran; it may have been edited since, or rolled back, and a
// model given a long history is inclined to trust what it already holds. This
// freshness check belongs at the resume boundary, not after each own save.
// It is fixed text, authored here, so it can sit in the user's message like the
// selection notice does — nothing in it comes from the artifact.
var resumedNotice = renderPrompt("resumed_notice", nil)
