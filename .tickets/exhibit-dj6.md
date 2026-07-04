---
id: exhibit-dj6
status: closed
deps: [exhibit-aq5]
links: []
created: 2026-05-31T22:30:50Z
type: feature
priority: 1
---
# Storage shim JS: localStorage/sessionStorage intercept

Vanilla JS shim that replaces localStorage/sessionStorage in the iframe. Hydrates from GET /api/artifacts/:id/state on load, serves getItem from cache, setItem writes through async. Bundle with esbuild.


