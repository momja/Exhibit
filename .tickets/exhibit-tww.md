---
id: exhibit-tww
status: open
deps: []
links: []
created: 2026-07-01T05:08:18Z
type: epic
priority: 2
---
# Tag management: colored pills, global rename/delete, per-artifact tagging UI

Extend the tags feature from bare create/list/attach/detach into a full tagging experience: tags carry a color, can be renamed and deleted globally (one place, propagates everywhere), and are managed directly from the gallery via colored pills with inline edit/add/remove controls.

CURRENT STATE (already built in schema v1): tags(id, owner_id, name) + artifact_tags(artifact_id, tag_id) join table exist; store has CreateTag/ListTags/AddArtifactTag/RemoveArtifactTag; API exposes /api/tags CRUD + artifact-scoped attach/detach; tests exist. What is MISSING and this epic adds: a color column, a global UPDATE (rename+recolor) path, a global DELETE path, hardened owner-scoped validation ('proper checks'), per-artifact tag hydration for the gallery, and all of the pill UI.

Split into two sub-epics: Backend (schema+store+API) and UI (gallery pills + modals). UI depends on Backend.

DECISIONS: color stored as hex TEXT NOT NULL DEFAULT '#6B7280', migration backfills existing rows; tag names UNIQUE(owner_id, name); delete is global and cascades to artifact_tags; pill 'x' detaches from one artifact (distinct from global delete); owner_id retained (always 1 for now, multi-user seam per PRD 4.4); timestamps explicitly OUT of scope. New UI uses Phosphor icons.

## Acceptance Criteria

Tags render as colored pills on gallery cards; a per-pill pencil opens an edit modal (rename/recolor/delete global), a per-pill 'x' detaches the tag from that artifact, and a trailing '+' opens an add modal (pick existing or create new). Global rename/recolor/delete propagate everywhere. All tag mutations are owner-scoped with proper validation and status codes.

## Notes

COLOR PALETTE DECISION (confirmed): tags default to a curated preset palette of a handful of reasonable, visually distinct colors (implementer's choice) shown as swatches in the edit/add modals, plus a custom hex entry for anything outside the presets. Not a locked/fixed palette — presets are the fast path, custom is always available. New tags default to '#6B7280' (neutral gray) until a color is chosen. Pill text auto-contrasts (black/white by luminance).


