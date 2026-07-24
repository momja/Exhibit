---
id: av-yvtb
status: open
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

