---
id: av-ombn
status: open
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

- A body size limit is enforced on every mutating route, returning 413 rather than buffering.
- The server is constructed with explicit ReadTimeout / ReadHeaderTimeout / MaxHeaderBytes instead of bare ListenAndServe.
- The limit is documented alongside the ingest limits and is large enough for a legitimately vendored artifact.
- A test asserts an over-limit body is rejected without being fully read into memory.

