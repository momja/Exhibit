---
id: exhibit-tww.1.3
status: closed
deps: []
links: []
created: 2026-07-01T05:09:09Z
type: task
priority: 2
parent: exhibit-tww.1
---
# Global tag delete: DeleteTag store + DELETE /api/tags/{id} (cascade)

Allow deleting a tag globally; its artifact associations are removed automatically via artifact_tags ON DELETE CASCADE (already in schema). Add Store.DeleteTag(ctx, ownerID, id) and DELETE /api/tags/{id} handler + route. Owner-scoped; unknown/foreign id -> 404. Distinct from the artifact-scoped detach (RemoveArtifactTag), which stays.

## Acceptance Criteria

DELETE /api/tags/{id} removes the tag and all its artifact_tags rows; other owners' tags untouched; missing -> 404; returns 204. Store + handler tests, including that associations are gone afterward.


