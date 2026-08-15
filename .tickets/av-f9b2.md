---
id: av-f9b2
status: in_progress
deps: []
links: [av-ghvs]
created: 2026-08-12T02:51:45Z
type: chore
priority: 2
assignee: Max Omdal
tags: [performance, render, api]
---
# No response compression: everything is served uncompressed

There is no `Content-Encoding`, gzip or brotli anywhere in `internal/` or `cmd/` — every response goes over the wire raw, including artifact render documents.

Measured on a 12.2 MB wasm payload: gzip takes it to 4.59 MB, ~2.7x. That is a larger win than any of the encoding choices debated in av-ghvs, and it applies to every artifact, not just vendored ones.

One wrinkle worth designing around: the render document is `Cache-Control: no-store` and composed per request, so naive on-the-fly compression re-compresses the whole body on every view — real CPU for a large artifact, with nothing cacheable to show for it. Consider compressing at rest (store the vendored body compressed and serve precompressed bytes when the client accepts it) rather than per request, or at minimum bound which responses get compressed on the fly.

## Acceptance Criteria

- Responses are compressed when the client advertises support, with a documented decision on on-the-fly vs at-rest compression for large render documents.
- Measured before/after byte counts for a representative large artifact are recorded on the ticket.
- Compression does not change the per-artifact CSP or any caching header semantics.


## Notes

**2026-08-15T19:32:19Z**

Branch coordination: merge/av-ghvs-av-f9b2 exists only as a deploy vehicle for testing av-ghvs + av-f9b2 together; delete it once both land. It is not a PR.
