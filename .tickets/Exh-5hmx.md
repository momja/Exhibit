---
id: Exh-5hmx
status: in_progress
deps: []
links: []
created: 2026-10-19T16:05:35Z
type: bug
priority: 1
assignee: Max Omdal
---
# Apply direct-open confirmation to the agent view

PR #163 missed the agent preview Open link. Route agent-view direct opens through the shared confirmation, preserving per-viewer/artifact remembered choices and supporting preview swaps after saves and artifact changes.

## Acceptance Criteria

Agent Open shows the shared confirmation; cancellation opens nothing; confirmation uses the current fresh-token /open route with noopener; remembered choices match the detail view; swaps and newly created artifacts work; focused tests and screenshots verify the behavior.

