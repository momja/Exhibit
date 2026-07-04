---
id: exhibit-tww.2
status: open
deps: [exhibit-tww.1]
links: []
created: 2026-07-01T05:08:36Z
type: epic
priority: 2
parent: exhibit-tww
---
# Tags UI: colored pills + edit/add modals in gallery (Phosphor icons)

UI sub-epic of exhibit-tww. Renders tags as colored pills on gallery cards and provides inline management. Depends on the Backend sub-epic (needs color, update/delete endpoints, and per-artifact tag hydration).

Pill anatomy: hover reveals a pencil icon on the LEFT (opens edit-tag modal: rename / change color / delete, global) and an 'x' on the RIGHT (detach this tag from this artifact only). A trailing '+' after the pills opens an add-tag modal that first shows a dropdown to select an existing tag OR create a new one (name + color). All new icons use Phosphor.

Built within the existing inline HTML render in internal/api/gallery.go (renderGalleryPage) using vanilla JS + fetch to the API, consistent with the current allowlist-editor pattern. Pill text color auto-contrasts against the tag color.

## Acceptance Criteria

Gallery cards show colored tag pills; pencil-on-hover opens a working edit modal (rename/recolor/delete with confirm) that propagates globally; 'x' detaches from the single artifact; '+' opens an add modal (select existing or create new) that attaches the tag. All controls use Phosphor icons and reflect changes without a full manual reload.


