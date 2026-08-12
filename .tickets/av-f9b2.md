---
id: av-f9b2
status: closed
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

One wrinkle worth designing around: the render document is `Cache-Control: no-store` and composed per request, so naive on-the-fly compression re-compresses the whole body on every view — real CPU for a large artifact, with nothing cacheable to show for it. Consider compressing at rest (store the vendored body compressed and serve precompressed bytes when the client accepts it) rather than per request, or at minimum bound which responses get compressed on the fly. Compressing at rest must NOT turn the stored blob into the response path directly: the render flow composes per-request state and the per-artifact CSP into the document before serving, so either cache the fully composed document separately, or decompress before composition and re-compress the composed output.

## Acceptance Criteria

- Responses are compressed when the client advertises support, with a documented decision on on-the-fly vs at-rest compression for large render documents.
- Measured before/after byte counts for a representative large artifact are recorded on the ticket.
- Compression does not change the per-artifact CSP or any caching header semantics.
- Responses negotiated by `Accept-Encoding` carry `Vary: Accept-Encoding`, and clients that advertise no supported encoding still receive the uncompressed body — both pinned by tests.


## Notes

**2026-08-15T19:32:19Z**

Branch coordination: merge/av-ghvs-av-f9b2 exists only as a deploy vehicle for testing av-ghvs + av-f9b2 together; delete it once both land. It is not a PR.

**2026-08-12T06:51:50Z**

Added gzip via chi middleware.Compress on both the app and render routers.

Measured on the real payload (the pokeemerald artifact with its 12 MB wasm vendored inline):
  uncompressed  16,376,525 bytes
  gzip           5,920,572 bytes   = 2.77x

Compression CPU is ~340 ms per render on that document (0.048s -> 0.386s over loopback,
so effectively all CPU). That is a per-request cost, not a one-off, because a render
document is composed per request and served no-store — which is exactly why level 5 was
chosen over 9.

Deliberate choices:
- Explicit content-type allowlist rather than chi's default list. text/event-stream MUST
  stay out of it: the agent surface streams SSE and asserts http.Flusher (agent.go:301);
  a buffering encoder would stall the stream. Already-compressed types (png, woff2, wasm)
  are absent because gzipping them spends CPU to add bytes.
- gzip only. Brotli would compress better but means a new dependency; stdlib gzip gets
  the bulk of the win.
- Middleware sits inside Recoverer so panic recovery stays outside the compressor.

Tests (internal/api/compression_test.go): render document comes back gzip and round-trips
to the same bytes, is under half the raw size, is uncompressed when the client does not
advertise support, an SSE-shaped handler stays uncompressed AND still satisfies the
http.Flusher assertion, and binary types are absent from the allowlist.

Not addressed here: compression is recomputed per view. Compressing at rest and serving
precompressed bytes would remove that CPU, and is the natural follow-up if it ever shows
up in profiles.
