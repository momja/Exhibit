---
id: exhibit-i0k
status: closed
deps: []
links: []
created: 2026-06-30T16:00:08Z
type: bug
priority: 1
assignee: Max Omdal
---
# URL/paste ingest does not persist scanned origins to allowlist

createArtifact scans the body for network origins (footprint) and returns it in the response, but stores req.NetworkAllowlist instead — which the gallery UI always sends as []. So detected origins are never written to the artifact's network_allowlist, the render CSP stays connect-src 'none', and the UI's claim that origins are 'already in the allowlist' is false. Fix: seed the allowlist from the scanned footprint when no explicit allowlist is provided, mirroring the PATCH/update path.


