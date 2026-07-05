---
id: av-xba2
status: closed
deps: []
links: [av-buyx]
created: 2026-07-04T23:26:00Z
type: bug
priority: 1
assignee: Max Omdal
---
# Storage shim state blocked by CSP/auth/CORS — artifacts don't persist

Render CSP connect-src omits the app origin, so the storage shim's hydrate/write-through fetches to /api/artifacts/:id/state are blocked by CSP. Additionally the state routes require the app auth token (unavailable in the sandboxed iframe) and the service sets no CORS headers for the cross-origin render->app call. Result: localStorage never persists across visits. Fix: add appOrigin to connect-src, make /state routes public, add CORS for the render origin.


## Notes

**2026-07-04T23:33:17Z**

Fixed on branch bug/av-xba2/shim-state-csp (commit 1e5f82d), deployed to test (aphrodite) and verified live:
- CSP connect-src now includes app origin: 'connect-src https://exhibit.maxomdal.com https://unpkg.com ...'
- GET/PUT /state reachable without auth token; CORS preflight returns allow-origin=render origin, methods GET,PUT,OPTIONS
- End-to-end: localStorage.setItem in the iframe wrote through to server; after full reload getItem hydrated the value back (verify-1783207888173). Zero CSP violations.
Ready for PR review; not merged.

**2026-07-04T23:45:22Z**

FOLLOW-UP after live test on the real Ticket Visualizer artifact:
- Write-through IS fixed: tkgraph:config:v1 (incl. github url/token, lastSource=github) now persists to server.
- But UI still shows 'No source connected' on reload. Root cause: HYDRATION RACE. The shim hydrates its cache via an async fetch that has NOT resolved when the artifact runs loadConfig() -> localStorage.getItem() synchronously at startup. getItem returns null -> auto-reconnect skipped. Seconds later getItem returns the config, but the artifact already decided.
- So the CSP/CORS/public-route fix is necessary but NOT sufficient; the shim's sync-getItem-over-async-hydrate design loses the startup read.
- Proposed real fix: render surface inlines the artifact's state directly into the shim (var cache = {..}) so getItem is correct on first synchronous read (no fetch, no race). Then revert GET /state to authenticated (no longer needed for hydration) and keep only PUT /state public+CORS for write-through.
- SECURITY: this artifact stores a GitHub PAT in localStorage -> synced to server in plaintext. My public GET /state change made it fetchable by URL with no auth. Token must be rotated. Reconsider public GET.

**2026-07-05T00:52:48Z**

VERIFIED FIXED (commit 92acd4e, deployed to aphrodite). Live test of the real Ticket Visualizer: fresh load of the render doc now auto-reconnects to GitHub (momja/Exhibit@main) and renders the graph — footer shows 24 open / 0 in progress / 28 closed / 52 total, empty-state hidden. Root cause (async-hydration race) resolved by inlining state into the shim at render time; deployed render doc confirmed to emit 'var cache = {...tkgraph:config:v1...}' with 0 async-hydrate fetches. Functional persistence complete. Encryption/public-GET hardening deferred to av-buyx.

**2026-07-05T01:51:03Z**

postMessage bridge VERIFIED in the real sandboxed gallery iframe (commit 04b8677):
- READ/inline hydration works in the opaque sandbox: embedded artifact reconnected to momja/Exhibit@main and rendered the full graph (24 open/28 closed/52 tickets). Confirmed shim overrides localStorage in an opaque-origin iframe (controlled test: defineProperty OK, getItem returns value).
- WRITE bridge works: iframe setItem -> postMessage(pinned to appOrigin) -> host listener -> same-origin authed PUT /state. Host PUTs observed firing on reconnect + filter toggles.
- Persistence + auth: authed PUT->204, GET reads back; unauth GET->401 (public endpoint exposure closed, per av-buyx).

CAVEATS / not-code issues found during verification:
1. TEST SERVER INSTABILITY: aphrodite was reverted from my branch to main mid-test (deployed source had old code despite my deploy), and PUTs intermittently 503 during container churn. No CI in repo and no auto-deploy in the ansible playbook (COPY . . build from synced source) — so an EXTERNAL process/deploy is pushing main to aphrodite and fighting the manual deploy. Needs the operator to identify/stop it. After redeploy it's stable (8/8 200 serving postMessage).
2. Render doc has NO Cache-Control header -> stale iframe docs possible. Recommend adding Cache-Control: no-store to render responses (dynamic per-artifact doc w/ inlined live state). Not yet done.
3. lastSource had drifted to 'local' (local-folder handle in IndexedDB, not synced -> can't auto-reconnect by design); restored to github for the test.

**2026-07-05T06:02:56Z**

Docs updated (PRD 5.3/5/8, architecture 3.1/3.2/6, tech-stack 2/4/6, README) to describe inline-read + postMessage-write. Added Cache-Control: no-store on render docs (+test). Removed dead web/static/shim.js. Deployed & verified live (cache-control: no-store, serving postMessage). PR #16 opened against main: https://github.com/momja/Exhibit/pull/16
