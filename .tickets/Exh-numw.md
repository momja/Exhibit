---
id: Exh-numw
status: open
deps: [Exh-imom]
links: []
created: 2026-07-11T05:02:35Z
type: task
priority: 2
assignee: Max Omdal
parent: Exh-avau
---
# Static gallery index: browsable grid + tag/collection pages

Emit a static equivalent of the live gallery index -- a browsable grid of artifacts with tag/collection groupings -- linking to each artifact's static page (Exh-imom). No live search (FTS5 is a runtime DB feature not available offline); browsing happens via the index and tag/collection listing pages themselves. Reuse the gallery template logic unblocked by epi-q0u2 rather than duplicating markup.

## Acceptance Criteria

The static build output includes an index page listing all built artifacts (grid, matching the live gallery's visual style) plus per-tag and per-collection listing pages, all linking to the correct static artifact pages. No broken links between index/listing pages and artifact pages.


## Complexity

M
