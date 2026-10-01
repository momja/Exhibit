---
id: av-ombn
status: closed
deps: []
links: [av-ghvs]
created: 2026-08-12T02:51:45Z
type: bug
priority: 1
assignee: Max Omdal
tags: [api, security, limits]
---
# No request body size limit on any write route

There is no `http.MaxBytesReader` anywhere in `internal/` or `cmd/`, and no middleware touches `r.Body` — the chain is only request logging, Recoverer, auth and owner scoping. `cmd/server/main.go` uses bare `http.ListenAndServe`, so there is also no `ReadTimeout`, `ReadHeaderTimeout` or `MaxHeaderBytes`, and docker-compose exposes the process directly with no proxy in front.

Measured: `PATCH /api/artifacts/:id` accepted a 16.3 MB JSON body without complaint.

Peak memory is roughly an order of magnitude over the request body, because it is held simultaneously as the decoded string, the snapshot output string, the base-injected string, a []byte copy at Blob.Put, plus three separate html.Parse DOM trees (extractTitle, scanner, ExtractSearchText), each several times the source size.

This was a latent issue while artifacts were small text files. av-ghvs makes large bodies a supported path (a vendored wasm artifact is ~16 MB), so it is now load-bearing. Same exposure on POST /api/artifacts and the widget endpoints.

Related: av-4bzn (agent sessions have no resource bounds).

## Acceptance Criteria

- An explicit numeric body size limit is enforced on every mutating route, returning 413 before the handler consumes the body. The number and its rationale are recorded on the ticket — large enough for a legitimately vendored ~16.3 MB artifact, with headroom, and no larger than the memory story can afford.
- A table-driven test enumerates every POST/PATCH/widget-write route and asserts the over-limit 413 on each, so no mutating route can be added without deciding its limit (a single over-limit test can pass while another write route stays unbounded).
- The server is constructed with explicit ReadTimeout / ReadHeaderTimeout / MaxHeaderBytes instead of bare ListenAndServe.
- The limit is documented alongside the ingest limits and is large enough for a legitimately vendored artifact.
- A test asserts an over-limit body is rejected without being fully read into memory.


## Notes

**2026-10-01T03:12:51Z**

Limit: 32 MiB (33554432 bytes), env MAX_REQUEST_BODY_BYTES, one limit for every route.

Why this number. The largest legitimate body is ~16.3 MB: a snapshot that vendored a wasm runtime, exported to one file (av-vnkt) and pasted back. Since av-20fk a URL ingest keeps those payloads out of line, but a paste of the exported file still carries them. 32 MiB is about twice that, and the headroom is needed: JSON escaping is paid on top, and a Go client escapes <, > and & to six bytes, so a markup-heavy 16 MiB document arrives as a 30 MiB request (measured).

Why not more. Peak heap for a write measured at ~10x the request: 160 MiB for a 16 MiB base64-heavy body, 166 MiB for 16.8 MiB of script, 315 MiB for a 30 MiB markup body. A maximal write at the default is ~320 MiB, which fits the 1 GB fly.toml provisions beside one agent sidecar. Smaller machines lower the env var.

Why one limit, not per route. The document writes (artifact POST/PATCH, widget PUT) are the only routes that amplify. The rest decode a few fields, so a smaller ceiling on them would not lower the worst case.

Server: ReadHeaderTimeout 10s, ReadTimeout 5m (32 MiB at ~0.9 Mbit/s), IdleTimeout 2m, MaxHeaderBytes 64 KiB. WriteTimeout stays 0 because the agent SSE stream is one long response; measured that ReadTimeout does not cancel it on Go 1.26.

Agent tools: EXHIBIT_MAX_BODY_BYTES goes to the Pi sidecar so a tool refuses an oversized write before sending it. A 413 from the API or a proxy produces the same message, and a dropped connection names the size it was sending. Measured that Node fetch gets the 413 cleanly from Go's server, but threw EPIPE mid-upload against another server, which is why the pre-check exists.
