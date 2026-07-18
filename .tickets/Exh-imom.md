---
id: Exh-imom
status: open
deps: [Exh-pgtb]
links: []
created: 2026-07-11T05:02:26Z
type: task
priority: 2
assignee: Max Omdal
parent: Exh-avau
---
# Static per-artifact render output (meta-tag CSP, read-only baked state)

Reuse the render surface's document composition (storage shim injection + artifact body -- the same template/asset logic unblocked by epi-q0u2) to emit one static HTML file per artifact instead of a dynamic response. Two adaptations from the live render path per the epic's Design section: (1) CSP -- the live surface sets CSP via an HTTP header; static hosts generally can't set custom headers, so bake the same generated policy (derived from network_allowlist) into a <meta http-equiv="Content-Security-Policy"> tag in each document instead. (2) State -- the live shim inlines state at request time and bridges writes to the API; a static build has no API, so the shim still inlines each artifact's current state at build time (correct first read) but setItem becomes a no-op / memory-only for the page's lifetime -- no write-through attempted, no network call to a nonexistent API.

## Acceptance Criteria

Given an artifact from a real instance, the static build emits a single self-contained HTML file that: renders correctly with no server present; enforces the artifact's network_allowlist via a baked <meta> CSP tag equivalent in content to the live header-based policy; inlines the artifact's current state read-only (setItem does not throw or attempt network access, and does not silently pretend to persist). Verified against at least one tier-1 artifact with existing localStorage usage.


## Complexity

M
