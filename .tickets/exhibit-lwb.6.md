---
id: exhibit-lwb.6
status: closed
deps: [exhibit-lwb.3, exhibit-lwb.4, exhibit-lwb.5]
links: []
created: 2026-06-30T02:57:58Z
type: task
priority: 2
parent: exhibit-lwb
---
# Snapshot ingest flow, partial-failure report + tests

Wire the vendoring transform into the URL-ingest path (artifacts.go createArtifact) behind a 'snapshot' option, running after fetch and before Blob.put. Return a report of what was vendored vs. left residual, total inlined size, and any per-asset failures so the user sees a usable artifact even on partial failure. Fallback when snapshot is off/fails: option A's injected <base href>. Add in-process API integration tests + fixtures (page with relative css/js/img/font + a nested @import + a deliberately-failing asset) driving the flow through the real HTTP handler with an httptest fixture server.

Note: "in-process API integration tests" here means Go httptest-level tests through the real chi router — NOT the Playwright browser-automation suite tracked by the separate epic av-f3cp (which this story does not depend on).

## Acceptance Criteria

URL ingest with snapshot on produces a self-contained artifact + report; partial failures don't abort ingest; <base href> fallback path covered; in-process API integration tests (httptest fixture server, real handler) green.


