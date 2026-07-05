---
id: exhibit-tww.1
status: closed
deps: []
links: []
created: 2026-07-01T05:08:31Z
type: epic
priority: 2
parent: exhibit-tww
---
# Tags backend: color column, global update/delete, hardened API

Backend sub-epic of exhibit-tww. Adds everything the tagging UI needs from schema/store/API: a color column on tags, global update (rename+recolor) and delete paths, owner-scoped validation on all tag mutations, and per-artifact tag hydration (id/name/color) for the gallery. No UI work here.

Existing surface to extend: internal/store/store.go (Tag struct, Store interface), internal/store/sqlite.go (CreateTag/ListTags/AddArtifactTag/RemoveArtifactTag), internal/api/collections.go (handlers), internal/api/api.go (routes), internal/store/migrations/001_initial.sql.

## Acceptance Criteria

tags table has a color column with a backfilled default; PATCH and DELETE /api/tags/{id} work and are owner-scoped; attach/detach/create validate existence + ownership + name uniqueness and return correct status codes; ListArtifacts returns each artifact's tags with color. Covered by tests.


