---
id: Exh-q6xo
status: in_progress
deps: [Exh-l2xj]
links: []
created: 2026-10-27T18:33:48Z
type: feature
priority: 2
assignee: Max Omdal
parent: Exh-17a5
tags: [agent, mcp]
---
# Stdio MCP server for one artifact

A local MCP server (stdio) that an agent launches with EXHIBIT_URL and EXHIBIT_TOKEN (a persisted grant from the previous story).

- Ship as a subcommand of the server binary (e.g. `server mcp`) or a small separate cmd; decide in design. Official Go SDK (github.com/modelcontextprotocol/go-sdk) unless there is a reason not to.
- Tools mirror exhibit.ts minus what does not apply: get_artifact, write_artifact, edit_artifact, get_state, set_state, delete_state, get_widget, set_widget, edit_widget. No create_artifact, no get_selection, no ask_user. No tool takes an artifact id.
- Writes carry the revision token from the last read; a 409 becomes a tool error telling the agent to re-read. Never retry automatically.
- Tool descriptions carry the guidance the Pi system prompt gives today (single-file artifact, storage shim, widgets), shared from one source so the two cannot drift.
- 413 and 403 surface as readable tool errors.

## Acceptance Criteria

Claude Code configured with the stdio server can read, edit and save an artifact; the save appears as an agent version; a concurrent edit in the browser yields a 409 tool error rather than a lost write.

