---
id: av-5aje
status: open
deps: []
links: [exhibit-x87, exhibit-fr7, av-kmwj]
created: 2026-07-20T05:51:26Z
type: feature
priority: 2
assignee: Max Omdal
tags: [scanner, ingest, pyodide]
---
# Pyodide runtime profile: one-approval network footprint at ingest

Pyodide artifacts generate their network footprint at runtime: loadPyodide() fetches wasm, python_stdlib.zip and the lockfile from the indexURL origin, loadPackage() pulls repacked wheels from the same origin, and micropip.install() hits pypi.org (metadata) and files.pythonhosted.org (wheels). The static ingest scan misses nearly all of this. Today each missing origin costs one Allow click in the av-kmwj runtime prompt plus a transparent reload, usually about two prompts (pypi.org and files.pythonhosted.org; the script origin is already scanned). The prompt needs trusted app chrome, so top-level and share renders get no prompt at all and stay blocked.

Key insight: for the default PyPI path, micropip's ORIGIN set is closed and known regardless of which packages get installed (dynamic names, transitive deps, lazy or interaction-gated installs all stay on pypi.org + files.pythonhosted.org). So a runtime profile can pre-fill the existing ingest approval screen: no probe sandbox, no extra approval moments. This is not a security guarantee: custom indexes and direct wheel URLs leave that set (below).

The branch feature/av-5aje/runtime-network-profiles held a standalone design doc. It was dropped on 2026-10-15 because docs/ describes only what is built; its content is folded in here.

## Design

- Scanner (internal/scanner) detects Pyodide signatures at ingest: loadPyodide(, a pyodide script src, 'import micropip', a literal indexURL.
- On detection the approval screen suggests a fixed bundle labeled 'Pyodide runtime + Python package installs': { indexURL origin, pypi.org, files.pythonhosted.org }. Use a literal indexURL's origin when present; otherwise derive it from the Pyodide script's URL.
- web/gallery/new.js already pre-checks every scanned origin in a flat list. What is new is runtime detection, the extra suggested origins, and their label. Deduplicate the bundle against scanned origins, and mark profile-suggested origins as such in the network_footprint response so they are distinguishable from scanned references. Otherwise "the scan is transparency" quietly becomes "the scan plus a hardcoded list".
- The scan stays transparency-only. Origins become decision='allow' rows in artifact_network_origins ONLY through explicit user approval, and the CSP stays the wall.
- Edge cases: the literal-URL heuristic reads only fetch( and import/from literals (scanner.go), so it does NOT see micropip.install('https://host/x.whl'), index_urls= or a JS indexURL property, even when they are literals. Either add explicit extraction for them or let them fall to the av-kmwj runtime prompt.
- Implement as a small runtime-profile table (signature -> known origin bundle) with Pyodide as the first entry. Add ruby.wasm, php-wasm or JupyterLite profiles only if demand shows up. Runtimes compiled to a single .wasm with literal references (Go/Rust via wasm_exec.js, SQLite-wasm) need no profile; the existing scan and the av-ghvs runtime-asset pass already cover them.

Non-goals:
- No runtime probe at ingest (a hidden iframe observing real requests, with a fixpoint-deepening CSP loop). It adds a second rendering path with its own CSP and approval UX.
- No Pyodide-specific runtime or package mirror. Existing snapshot vendoring (av-ghvs, av-20fk) stays as is; it does not reach Pyodide because its boot and loadPackage URLs are derived from indexURL, not fetch() literals, so the asset manifest never sees them. Hosting wheels would make the service a package mirror (per-user storage or content-addressed dedup both end in bandwidth, availability and supply-chain responsibility on our server) and breaks the light-server property that keeps hosted instances cheap. Pyodide artifacts leaning on CDN and PyPI uptime is an accepted, documented durability property of that artifact class.

## Acceptance Criteria

1) Ingesting a Pyodide artifact shows the approval screen with the Pyodide bundle pre-checked and labeled as profile-suggested. 2) Approving once at ingest renders the artifact with loadPyodide boot, loadPackage() and default-index micropip.install() all working with zero further prompts. 3) Non-Pyodide artifacts are unchanged (no bundle suggested). 4) The profile never auto-approves: origins persist only through explicit user approval. 5) The implemented profile behavior and its limits are documented in docs/architecture.md §3.4 and docs/security.md, landed with the implementation.
