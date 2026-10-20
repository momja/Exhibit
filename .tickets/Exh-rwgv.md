---
id: Exh-rwgv
status: in_progress
deps: []
links: []
created: 2026-10-20T06:06:27Z
type: bug
priority: 2
assignee: Max Omdal
---
# Save widget edits with the artifact Save button

The artifact edit page has a separate Save widget button, while the bottom Save button leaves widget edits unsaved. Make the bottom Save action persist the widget before reporting success or leaving the page, preserve conflict/error handling, and remove the redundant widget save button.

## Acceptance Criteria

Bottom Save persists artifact and nonempty widget edits; empty default widgets remain absent; widget failures preserve edits and prevent navigation; the separate Save widget button is removed.

