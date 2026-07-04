---
id: exhibit-tww.1.2
status: open
deps: [exhibit-tww.1.1]
links: []
created: 2026-07-01T05:09:08Z
type: task
priority: 2
parent: exhibit-tww.1
---
# Global tag update: rename + recolor (UpdateTag store + PATCH /api/tags/{id})

Enable editing a tag in ONE place so the change propagates to every artifact (PRD rationale for a tags table). Add Store.UpdateTag(ctx, t *Tag) (or (id, name, color)) updating name+color for the owner's tag; add PATCH /api/tags/{id} handler + route in api.go. Owner-scoped (only the owner's tag). Enforce UNIQUE(owner_id, name): renaming to a colliding name returns 409. Unknown id returns 404.

## Acceptance Criteria

PATCH /api/tags/{id} with {name?,color?} updates the tag; the new name/color is visible on every artifact that has it; owner scoping enforced; duplicate name -> 409; missing tag -> 404. Store + handler tests included.


