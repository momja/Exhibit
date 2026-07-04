---
id: exhibit-lwb.6
status: open
deps: [exhibit-lwb.3, exhibit-lwb.4, exhibit-lwb.5]
links: []
created: 2026-06-30T02:57:58Z
type: task
priority: 2
parent: exhibit-lwb
---
# Snapshot ingest flow, partial-failure report + tests

Wire the vendoring transform into the URL-ingest path (artifacts.go createArtifact) behind a 'snapshot' option, running after fetch and before Blob.put. Return a report of what was vendored vs. left residual, total inlined size, and any per-asset failures so the user sees a usable artifact even on partial failure. Fallback when snapshot is off/fails: option A's injected <base href>. Add end-to-end tests + fixtures (page with relative css/js/img/font + a nested @import + a deliberately-failing asset).

## Acceptance Criteria

URL ingest with snapshot on produces a self-contained artifact + report; partial failures don't abort ingest; <base href> fallback path covered; e2e tests green.


