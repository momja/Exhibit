---
id: Exh-edru
status: in_progress
deps: []
links: []
created: 2026-10-21T18:10:46Z
type: bug
priority: 2
assignee: Max Omdal
---
# Separate new-tag creation from existing-tag selection on edit page

The edit-page tag creator depends on selecting Create new tag in the existing-tag dropdown, even while the name and color fields are visible. Provide an independent creation control and explicit create-and-attach action.

## Acceptance Criteria

Existing tags attach through their own selector and button. New tags can be created and attached without any dropdown selection. Validation is scoped to the relevant action; tag changes refresh through htmx; desktop and mobile controls remain usable.

