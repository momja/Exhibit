---
id: av-7t1e
status: in_progress
deps: []
links: []
created: 2026-09-25T15:39:41Z
type: feature
priority: 2
assignee: Max Omdal
tags: [agent]
---
# Agent widget edit support (edit_widget tool)

Give the agent sidecar's exhibit.ts an edit_widget tool modeled on edit_artifact (av-f5i5): targeted {oldText, newText} edits applied to the current widget source, same validation (exact-first/fuzzy fallback, unique, non-overlapping, no-PUT on invalid), persisted through the same PUT + unapproved_origins + widget_saved path as set_widget, which remains for full tile rewrites.

## Acceptance Criteria

edit_widget registered with same scoping (no id); exact/multi/fuzzy edits work; invalid edits fail with no PUT; missing widget reports to use set_widget; widget_saved event fires so the tile refreshes; prompt + docs + chat label updated

