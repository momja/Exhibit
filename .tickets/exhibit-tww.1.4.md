---
id: exhibit-tww.1.4
status: open
deps: [exhibit-tww.1.1]
links: []
created: 2026-07-01T05:09:09Z
type: task
priority: 2
parent: exhibit-tww.1
---
# Harden tag mutations: existence, owner-scoping, name uniqueness, status codes

The 'proper checks' gap. Today addArtifactTag/removeArtifactTag blindly INSERT OR IGNORE / DELETE with no validation, and createTag allows duplicate names. Harden: (1) attach validates the artifact and tag both exist and belong to the owner before inserting -> 404 otherwise; (2) detach returns 404 if the pairing didn't exist (currently always 204); (3) createTag enforces UNIQUE(owner_id,name) — add the constraint (in migration 002 or a follow-on) and return the existing tag or 409 on conflict instead of silently duplicating. Consistent JSON error bodies + status codes across tag handlers.

## Acceptance Criteria

Attaching a nonexistent/foreign artifact or tag -> 404; detaching a non-association -> 404; creating a duplicate name is rejected/deduped (no duplicate rows); all tag endpoints owner-scoped. Table-driven handler tests cover the failure paths.


