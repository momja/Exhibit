---
id: av-yvtb
status: in_progress
deps: []
links: [av-x01o]
created: 2026-07-24T04:37:58Z
type: bug
priority: 2
assignee: Max Omdal
tags: [security, render, sandbox]
---
# Module workers can't run in the opaque-origin sandbox iframe (blocks ffmpeg.wasm in the gallery preview)

Verified in Chrome against a local build with the av-x01o CSP fix in place: inside the gallery's sandboxed iframe (sandbox=allow-scripts, no allow-same-origin, opaque 'null' origin), a Worker constructed from a blob: URL with {type:'module'} fires onerror with an empty message and never runs. Classic blob: workers and data: workers run fine in the same frame, and the same module worker runs fine when the artifact is opened top-level at RENDER_ORIGIN/a/:id. No securitypolicyviolation event fires, so this is not the CSP -- it is the browser refusing module-script fetches for an opaque origin.

Practical impact: ffmpeg.wasm 0.12 creates its class worker as {type:'module'} whenever classWorkerURL is passed (the only way to spawn it from a cross-origin CDN), so an ffmpeg artifact loads and transcodes correctly when opened in a new tab or via a share link, but hangs forever on 'loading core...' in the gallery's embedded preview. Same silent-hang signature as av-x01o, different cause.

Options to weigh (none obviously free): serve the render iframe from a real per-artifact subdomain so it is not an opaque origin (spec 12 'hardened' path, needs wildcard DNS/cert from the operator); surface the limitation in the preview UI with a 'open in new tab' nudge when a module worker fails; accept and document. Adding allow-same-origin is NOT an option -- it dissolves the trust boundary.

## Acceptance Criteria

Decide and document the stance. If we mitigate, an artifact using a module Worker either runs in the embedded preview or tells the user why it can't and offers the top-level render. Whatever is chosen is written down in docs/security.md next to the sandbox section.


## Notes

**2026-07-24T06:31:27Z**

DECISION (2026-07-23): pursue detect-and-warn, keep the opaque-origin sandbox. Reject per-artifact-subdomains + allow-same-origin as the DEFAULT.

Why option 1 is rejected as default (not just cost): the opaque origin does double duty — it is the security boundary AND the enforcement mechanism for "all state is server state." In a no-allow-same-origin frame the real localStorage throws, so the storage shim is the ONLY possible store and cross-device is airtight. allow-same-origin gives the artifact a real, disk-backed origin store; the shim still overrides window.localStorage, but any surface it does not cover (IndexedDB — shim still deferred) lands state per-device again. So the trade is a triangle: module workers require a real origin -> a real origin requires real local storage -> real local storage undercuts server-authoritative state. Option 1 costs the airtightness of cross-device, not merely a wildcard cert. It stays available only as an explicit HARDENED opt-in, never the default.

PLAN (phased):

Phase 1 — detect & warn (render preamble, framed-only). Intercept the Worker constructor; when it is called with {type:'module'} while location.origin === 'null' (the opaque sandbox), postMessage a diagnostic to the host frame. The host shows a non-blocking banner on the preview: browsers block module workers in the embedded sandbox; offer "Open in new tab" (the top-level render runs the module worker fine). Debounce to first occurrence. This converts the silent, indefinite "Loading..." hang into an explained, actionable state. Classic blob:/data: workers are unaffected — they run in the sandbox after av-x01o; only module workers trip this. Possible extension: same treatment for SharedWorker and service-worker registration, which also fail on an opaque origin.

Phase 2 — agent-assisted rewrite. Surface a "Fix with agent" action that hands the diagnostic (this artifact uses a module worker) to the agent sidecar (internal/agent); the agent attempts a sandbox-compatible rewrite — a classic worker driven by importScripts, or main-thread wasm — via its update_artifact tool. Output is rescanned like any ingest; footprint is never auto-approved.

Explicitly NOT doing: allow-same-origin on a shared render origin (dissolves cross-artifact isolation); routing artifacts to top-level to "fix" it (loses the sandbox layer — note the per-artifact CSP travels with the document regardless, so top-level keeps the network wall but drops origin isolation).
