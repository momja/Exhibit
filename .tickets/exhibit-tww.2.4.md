---
id: exhibit-tww.2.4
status: open
deps: [exhibit-tww.1.2, exhibit-tww.1.3, exhibit-tww.2.3]
links: []
created: 2026-07-01T05:09:45Z
type: task
priority: 2
parent: exhibit-tww.2
---
# Edit-tag modal: rename, change color, delete (global)

Modal opened by the pill pencil. Fields: name (text) and color (preset palette + custom hex), plus a Delete action (Phosphor trash, with a confirm step). Save -> PATCH /api/tags/{id} (tww.1.2); Delete -> DELETE /api/tags/{id} (tww.1.3). Because these are GLOBAL, on success refresh the gallery so every pill of that tag updates/disappears (htmx/partial or targeted re-render). Reuse this modal shell for the add flow (tww.2.5). Duplicate-name (409) surfaces an inline error.

## Acceptance Criteria

Editing name/color updates the tag on every artifact that has it; delete (after confirm) removes the tag everywhere; duplicate name shows an error and doesn't save; modal closes on success and the gallery reflects changes without a manual full reload.


