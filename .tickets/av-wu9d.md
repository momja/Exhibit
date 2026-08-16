---
id: av-wu9d
status: open
deps: []
links: [av-ghvs]
created: 2026-08-12T02:51:46Z
type: bug
priority: 3
assignee: Max Omdal
tags: [ingest, api]
---
# PATCH uses Scan instead of ScanWithBase for URL-ingested artifacts

The body-rewrite path in `PATCH /api/artifacts/:id` (internal/api/artifacts.go) calls `scanner.Scan` on both the new and old body, even when the artifact has a `SourceURL`. `Scan` drops relative references; `ScanWithBase` resolves them.

So editing a URL-ingested artifact reports a smaller footprint than its own ingest did, and `footprint_changed` is computed from that inconsistent view — the edit dialog's approval gate can conclude nothing changed when the relative references tell a different story.

Minor related issue in the same block: when reading the previous blob fails the error is ignored, leaving oldBody empty, which makes the diff trivially true and reports footprint_changed spuriously.

## Acceptance Criteria

- The PATCH body path uses ScanWithBase with the artifact's SourceURL when it has one, matching ingest.
- A failed read of the previous body aborts the PATCH before the new body is written — or records an explicit "unknown comparison" while preserving the existing blob and approval state. Either way the current silent-empty-baseline behavior (which makes the diff trivially true and reports footprint_changed spuriously) is gone.
- A test asserts an edit to a URL-ingested artifact reports the same footprint shape ingest did.

