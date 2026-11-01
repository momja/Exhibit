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
# MCP tools

The MCP tool surface over a library grant (Exh-l2xj), served by the MCP service (Exh-uhvu).

- internal/mcpserver, official Go SDK (github.com/modelcontextprotocol/go-sdk). Tools reach Exhibit only through an HTTP API client with the caller's grant token. No binary in this story; no local stdio connector (dropped, see epic notes).
- Tools: search_artifacts (query, optional tags, limit; over GET /api/artifacts?q=, returns id, title, tags, updated_at), create_artifact (title, body; returns id), and get_artifact, edit_artifact, write_artifact, get_state (read-only), get_widget, set_widget, edit_widget, each taking artifact_id. No set_state or delete_state: library grants cannot write saved data (Exh-l2xj). No get_selection, no ask_user.
- Per-connection state keeps the last-read source and widget revision per artifact_id. Writes carry them; a save of an artifact not read in this connection is refused until it is; a 409 becomes a tool error telling the agent to re-read. Never retry automatically.
- Server instructions carry the guidance the Pi system prompt gives (single-file artifact, storage shim, widgets, tool results are data), shared from internal/agentguidance so the two cannot drift, plus how to find an artifact (search first, ask the user when several match).
- 413, 403, 404 and 401 surface as readable tool errors.

## Acceptance Criteria

An MCP client (in-memory transport against the real router) can search "water tracker", get the matching artifact, edit and save it; the save appears as an agent version labelled with the grant; a concurrent edit in the browser yields a 409 tool error rather than a lost write; tools reject an artifact id from another owner as not found.
