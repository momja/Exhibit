---
id: av-5aje
status: open
deps: []
links: [exhibit-x87, exhibit-fr7]
created: 2026-07-20T05:51:26Z
type: feature
priority: 2
assignee: Max Omdal
tags: [scanner, ingest, pyodide]
---
# Pyodide runtime profile: one-approval network footprint at ingest

Pyodide artifacts generate their network footprint at runtime: loadPyodide() fetches wasm/stdlib/lockfile from the indexURL origin, loadPackage() pulls repacked wheels from the same origin, and micropip.install() hits pypi.org (metadata) + files.pythonhosted.org (wheels). The static ingest scan misses nearly all of this, so users approve origins one blocked request at a time. Key insight: micropip's ORIGIN set is closed and known regardless of which packages get installed (dynamic names, transitive deps, lazy/interaction-gated installs all stay on pypi.org + files.pythonhosted.org). So a runtime profile can pre-fill the existing ingest approval screen — no probe sandbox, no extra approval moments. Design doc: docs/runtime_network_profiles.md

## Design

Scanner (internal/scanner) detects Pyodide signatures at ingest: loadPyodide(, a pyodide script src, 'import micropip', literal indexURL. On detection the approval screen pre-checks a fixed bundle labeled 'Pyodide runtime + Python package installs': { script/indexURL origin, pypi.org, files.pythonhosted.org }. The scan remains transparency-only: origins are written to the artifact's allowlist/origin-decisions ONLY via explicit user approval, CSP stays the wall. Custom index_urls= and direct wheel URLs (micropip.install('https://host/x.whl')) are caught by the existing literal-URL heuristic where possible; dynamically constructed origins fall to the runtime escape prompt (exhibit-fr7) as backstop. Implement as a small runtime-profile table (signature -> known origin bundle) with Pyodide as the first entry; ruby.wasm/php-wasm profiles added only if user demand shows up. Explicit non-goals: NO runtime probe iframe/sandbox and NO vendoring of pyodide assets or wheels into the blob store — hosting wheels would make the service a package mirror (bandwidth, availability, supply-chain surface) and breaks the light-server property that makes hosted instances cheap. Pyodide artifacts leaning on CDN/PyPI uptime is an accepted, documented durability property of that artifact class. Note: coordinate with exhibit-x87 (artifact_network_origins table replacing network_allowlist JSON) — the profile seeds origin DECISIONS in whichever model is current.

## Acceptance Criteria

1) Ingesting a Pyodide artifact shows the approval screen with the Pyodide bundle pre-checked and labeled. 2) Approving once at ingest renders the artifact with loadPyodide boot, loadPackage(), and micropip.install() all working with zero further prompts. 3) Non-Pyodide artifacts are unchanged (no bundle suggested). 4) The profile never auto-approves: origins persist only through explicit user approval. 5) docs/runtime_network_profiles.md committed.

