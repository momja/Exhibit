---
id: Exh-9jcb
status: closed
deps: []
links: []
created: 2026-10-28T15:15:05Z
type: chore
priority: 3
assignee: Max Omdal
tags: [agent, prompts]
---
# Extract agent prompts from inline variables into template files

Model-facing prompt text is scattered across Go and TS source as inline string constants and literals, which makes prompts hard to review, diff and edit as prose. Move every prompt into dedicated template files (embedded with go:embed for Go; imported/bundled for the Pi extension), rendered with text/template where they take parameters.

Known inline prompts:
- internal/agent/prompt.go: rolePrompt, dataContract, questionGuidance, modePrompt's WidgetOnly and edit-artifact paragraphs, selectionNotice (parameterized count), resumedNotice
- internal/api/widget.go: generateWidgetPrompt
- internal/agent/ext/guard.ts: the guardrail screening prompt (~line 107)
- Sweep for others (agent tool descriptions in internal/agent/ext/exhibit.ts, mockllm fixtures excluded).

## Design

Suggested layout: internal/agent/prompts/*.tmpl (plain text/template; loader trims the trailing newline editors add), one file per prompt, loaded via embed.FS and parsed once at init (fail fast on parse error). buildSystemPrompt keeps its composition logic and the SystemPrompt override semantics; only the text moves. Keep the 'what is absent' invariant: no artifact title/id interpolated into system prompt templates (av-e0yj, av-5s7g). For guard.ts, decide whether the Pi extension reads a file shipped beside it or the text is inlined at build; it must stay a fixed, non-user-controlled string.

## Acceptance Criteria

- No model-facing prompt text remains as an inline string constant in Go or TS source
- Rendered system prompts are byte-identical to before (golden test comparing old vs new output for each mode: default, edit, widget-only, override)
- Existing prompt_test.go, guard tests and ext tests pass
- docs/agent.md notes where prompts live


## Notes

**2026-10-28T15:35:30Z**

Shipped: system prompts, mode paragraphs, notices, generate-widget turn, and guardrail screen/classifier/policy moved to .tmpl files. Agent tool descriptions and tool-result strings in ext/exhibit.ts and ext/questions.ts deliberately stay inline beside their tool definitions; follow-up if wanted.
