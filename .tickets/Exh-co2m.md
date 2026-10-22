---
id: Exh-co2m
status: in_progress
deps: []
links: [Exh-h5va, Exh-g2vv]
created: 2026-10-22T00:49:02Z
type: feature
priority: 2
assignee: Max Omdal
---
# POC: sandboxed artifact backends (wazero + QuickJS) with standalone bundle

Proof of concept for server-side artifact handlers. A Workers-style handler (export default { fetch(request, env) }) embedded in the artifact runs per request in a fresh QuickJS-in-WASM instance under wazero. Its only exits are host calls: fetch through one egress gate enforcing the artifact's network allowlist (the same allow rows that build the CSP) with a dial-time SSRF guard, and localStorage backed by the viewer's artifact_state rows. The frame reaches the handler with a plain relative fetch, routed by the shim over the host bridge. Demo: an arXiv explorer (arXiv's API sends no CORS headers). Export produces a standalone bundle runnable on the user's machine.

## Design

Throwaway-quality POC on a branch; not for merge as-is. Findings and measurements go in docs/backend-poc.md.

## Acceptance Criteria

1. A demo artifact with a backend fetches arXiv through the handler from the framed preview. 2. A non-allowlisted origin, loopback/private addresses, and runaway CPU/memory are refused or bounded, with tests. 3. A downloaded bundle runs on a user's machine and serves the same app. 4. Report with measurements and open questions.


## Notes

**2026-10-22T06:51:01Z**

Follow-up direction recorded in Exh-g2vv (experimental): park the server-side backend, build a credential-free relay plus connectors instead. Findings in docs/backend-poc.md on feature/Exh-co2m/backend-poc.
