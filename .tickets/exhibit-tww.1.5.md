---
id: exhibit-tww.1.5
status: open
deps: [exhibit-tww.1.1]
links: []
created: 2026-07-01T05:09:10Z
type: task
priority: 2
parent: exhibit-tww.1
---
# Hydrate per-artifact tags (id, name, color) for the gallery

The gallery needs each artifact's tags with color to render pills; ListArtifacts does not populate them today (store.Artifact.Tags is []string and unused on read). Populate artifact tags on list/get: either extend Artifact with a []Tag (id,name,color) field or add a batched Store.TagsForArtifacts([]id) to avoid N+1. Wire galleryIndex to pass tags to renderGalleryPage. Keep it a single efficient query (JOIN artifact_tags/tags), not per-card lookups.

## Acceptance Criteria

ListArtifacts / gallery data include each artifact's tags with id+name+color; no N+1 (one query for the page of artifacts); covered by a store test asserting the right tags come back per artifact.


