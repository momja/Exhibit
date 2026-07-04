---
id: exhibit-i4l
status: closed
deps: []
links: []
created: 2026-06-22T04:07:39Z
type: task
priority: 2
assignee: Max Omdal
---
# Unit tests for artifact editing + URL ingest

Commit a173681 added artifact editing (PATCH body, edit page) and URL-based ingest (extractTitle, URL fetch in createArtifact) under beads 6hh and chh, but shipped without dedicated unit tests. Add coverage for extractTitle, URL-fetch ingest path, PATCH body re-scan, and the galleryEdit handler.


