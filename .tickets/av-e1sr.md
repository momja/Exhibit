---
id: av-e1sr
status: open
deps: []
links: []
created: 2026-07-07T05:44:12Z
type: feature
priority: 2
assignee: Max Omdal
parent: av-f3cp
---
# e2e: test harness + state round-trip flagship test

Playwright workspace at e2e/ (top-level, deliberately outside web/ so scripts/build-assets.sh's web/*/package.json glob never sweeps test deps into the asset build). playwright.config.ts webServer harness: starts the built bin/server with a scratch DATA_DIR on dedicated ports 18080/18081 (two ports = two real origins). fixtures/counter.html (reads localStorage synchronously at startup, increments on click) + fixtures/seed.ts helper that ingests via POST /api/artifacts with the bearer token (single write path). Makefile gains e2e and e2e-ui targets. See docs/proposals/e2e_testing.md §3-§5.

## Acceptance Criteria

GIVEN a seeded counter.html artifact WHEN the visitor opens it, clicks twice, and reloads THEN the counter reads 2 after reload — provable only if state traveled shim -> postMessage -> host PUT /state -> server -> re-inlined at render (opaque-origin iframe + no-store means no client storage can satisfy it). Suite runs green via 'make e2e' from a clean checkout after 'make build'.


## Complexity

L
