---
id: av-lvi1
status: open
deps: [av-hg5f, av-9jll]
links: []
created: 2026-08-01T19:01:48Z
type: feature
priority: 3
assignee: Max Omdal
parent: av-p4hm
tags: [state, agent, api]
---
# Agent sidecar: read and edit artifact state

Give the agent sidecar tools to read and modify an artifact's stored state, so a user can say 'clear the saved settings on this one' or 'my todo list has a corrupted entry, fix it' in the chat surface instead of hand-correcting rows in the edit page's state inspector.

This is the conversational counterpart to av-hg5f: same data, same routes, different surface. It is filed separately and at lower priority because it depends on av-hg5f having defined the delete routes and, more importantly, on the shim's semantics being correct first — an agent editing state through a layer where removeItem leaves tombstones and clear() silently reverts would produce confidently wrong results.

## Design

Add get_state / set_state / delete_state tools to internal/agent/ext/exhibit.ts alongside the existing create_artifact / update_artifact / get_artifact, calling the same authenticated HTTP API with the service token. No new write path (architecture.md §3.7) — the sidecar keeps exactly the reach any other API client has.

Prompt-side: the system prompt needs to describe state as a typed key/value map and set the expectation that values are usually JSON, so the model round-trips them faithfully rather than reformatting. It must also carry whatever the sessionStorage split (av-9jll) settles on, since an agent told 'sessionStorage persists cross-device' will write artifacts against a contract that no longer holds.

Open question for implementation: whether a state edit should emit a synthetic event that refreshes the preview pane the way exhibit_artifact_saved does — state changes are inlined at render, so the pane is stale after an edit until something re-renders it.

## Acceptance Criteria

1. The agent can list an artifact's state keys and values.
2. The agent can set and delete individual keys, and erase all state, through the same API routes the edit page uses.
3. No new write path is introduced — every mutation goes through the authenticated HTTP API.
4. The system prompt describes state semantics accurately, consistent with whatever av-9jll ships.
5. Values that are JSON survive a read/write round-trip unchanged when the agent was not asked to modify them.

