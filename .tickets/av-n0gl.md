---
id: av-n0gl
status: open
deps: []
links: []
created: 2026-09-25T16:15:02Z
type: feature
priority: 2
assignee: Max Omdal
tags: [agent, api]
---
# Optimistic concurrency for artifact and widget writes

All body/tile writes are read-modify-write with no precondition: the agent edit/write tools GET then PATCH/PUT, and the human edit page and widget panel do the same. A concurrent save between the read and the write is silently clobbered — last writer wins with no signal. Add an optimistic-concurrency precondition (revision counter or content hash, not updated_at at 1s granularity) to PATCH /api/artifacts/:id body writes and PUT /api/artifacts/:id/widget, rejected with an explicit conflict when the precondition misses, so every writer (agent tools, edit page, widget panel, refetch) either wins cleanly or learns it lost. Agent tools must surface a conflict as a re-read-and-retry error, never an unbounded auto-retry loop.

## Acceptance Criteria

precondition supported on artifact body PATCH and widget PUT; mismatch returns an explicit conflict instead of overwriting; agent edit/write/widget tools translate a conflict into a re-read-and-retry tool error; human edit page and widget panel handle a conflict without silent data loss; concurrent-write tests cover both resources

