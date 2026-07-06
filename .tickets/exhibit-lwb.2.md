---
id: exhibit-lwb.2
status: closed
deps: [exhibit-lwb.1]
links: []
created: 2026-06-30T02:57:32Z
type: task
priority: 2
parent: exhibit-lwb
---
# Bounded asset resolver + fetcher

Core reusable component for vendoring. Given a source base URL and a relative/absolute/protocol-relative/root-relative reference, resolve it to an absolute URL and fetch the bytes. Must enforce strict limits: per-asset size cap, total-bytes cap, max asset count, timeout, and a redirect/SSRF guard (no fetching localhost/internal ranges). Dedupe identical URLs. Return bytes + content-type or a typed fetch error the caller can record without aborting the whole snapshot. Pulls all fetch-policy complexity into one place.

## Acceptance Criteria

resolves all four relative forms against a base; enforces size/count/timeout limits; blocks internal-address fetches; surfaces per-asset errors without panicking.


