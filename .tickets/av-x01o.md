---
id: av-x01o
status: in_progress
deps: []
links: [av-yvtb]
created: 2026-07-24T04:13:43Z
type: bug
priority: 1
assignee: Max Omdal
tags: [security, csp, render]
---
# Render CSP: make no-egress directives (worker-src, script-src blob:/data:/unsafe-eval) unconditional, not scan-gated

The render surface's generated CSP (internal/render/render.go, buildCSP) omits an explicit worker-src directive, relying on fallback to script-src. Empirically verified in a live browser against a real deployed artifact: Worker() objects (module workers, sourced from blob: or data: URLs) are created without error but their script body never executes — confirmed with three independent repros (trivial blob-sourced worker, trivial data:-URL worker, and a real FFmpeg.wasm worker), none of which ever ran a single line, not even a bare console.log. This silently hangs any artifact that uses Web Workers (e.g. FFmpeg.wasm-based tools), with no console error and no rejected promise, so it manifests as an indefinite 'Loading...' hang.

This is the second CSP-directive gap hit in ad-hoc succession, prompting a policy-level fix rather than another narrow patch.

Root cause: the CSP already draws a distinction between (a) directives that could reach the network, which are correctly scan+approve+allowlist gated per docs/product_requirement_doc.md §6.2, and (b) inlined/local, no-egress capabilities, which the doc explicitly says are 'exempt from allowlist approval' -- e.g. style-src always carries 'unsafe-inline', img-src/font-src always carry data:, media-src always carries blob:. worker-src (and script-src's blob:/data:/'unsafe-eval') belong in bucket (b): a worker instantiated from a blob:/data: URL cannot reach the network on its own -- it's local execution, not egress -- so gating it behind per-artifact scan/approval buys no security and breaks any artifact using Workers.

## Acceptance Criteria

buildCSP in internal/render/render.go emits an explicit worker-src directive that unconditionally includes blob: and data: (mirroring the existing unconditional blob:/data: grants on script-src/media-src/img-src/font-src), independent of the artifact's allowlist.
Reproduce the FFmpeg.wasm artifact (ab4fc00e-0d9f-4191-a120-778751b8aef1 on the maxomdal.com test deployment, or an equivalent local fixture) end-to-end: upload a video, confirm FFmpeg loads and export/download completes, not just that the worker fires a console.log.
Add/extend a render_test.go case asserting worker-src is present with blob: and data: even when the artifact's allowlist is empty (connect-src 'none' case), mirroring the existing pattern for media-src/img-src/font-src (see render_test.go:32-52).
Document the bucket distinction (network-reaching vs local/no-egress CSP directives) inline as a comment near buildCSP so future directive additions default to the correct bucket without re-litigating this.

