---
id: av-wu9d
status: closed
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


## Notes

**2026-09-26T16:33:24Z**

Decision (rule b): base counts nowhere, relatives resolve through the doc's own <base>. Implemented ScanDoc (own base governs, tag never listed) instead of the acceptance's unconditional ScanWithBase(SourceURL): unconditional misattributes Exhibit-authored locals to the source site and misses intentional base deletion. Save stays verbatim (no re-injection — authored paths are Exhibit-namespace). Shrinks-to-empty stays silent (no auto-revoke, explicit-only); refetch stays av-b17a scope.

**2026-09-26T17:05:21Z**

Review fixes: ingest now injects the fallback base first and scans the stored body with ScanDoc (one rule for ingest, PATCH, edit page and widgets, via networkFootprint, which also drops the render origin everywhere). ScanWithBase removed. docBase follows the HTML rule (first base with href wins even if empty; svg/template bases ignored; trimmed; protocol-relative takes https). PATCH: a missing previous body (fs.ErrNotExist, now promised by both blob backends; S3 maps NoSuchKey) proceeds as a repair with footprint_changed=true; any other read failure aborts before any write.
