---
id: av-11qx
status: closed
deps: []
links: []
created: 2026-07-05T16:22:59Z
type: task
priority: 2
assignee: Max Omdal
---
# create debug mode with verbose logging support so that developers can receive more rich feedback in test environments


## Notes

**2026-07-07T21:28:56Z**

Implemented in this branch (commit 30ab42c). Added internal/logging (log/slog leveled logger + RequestMiddleware wired into both app and render routers), a serverError() helper logging every 500 with operation label/method/path, and debug lifecycle traces at ingest/render/state/scanner/blob/snapshot seams. Gated by LOG_LEVEL / DEBUG env vars. Tests added in internal/logging and internal/api. Build, vet, gofmt, and full test suite pass.
