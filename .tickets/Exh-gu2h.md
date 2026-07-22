---
id: Exh-gu2h
status: closed
deps: []
links: []
created: 2026-07-21T05:23:28Z
type: bug
priority: 2
assignee: Max Omdal
tags: [renderer, csp, security]
---
# CSP media-src blocks blob: media playback (default-src none fallback)

Reported by user testing https://exhibit.maxomdal.com against an artifact that imports/plays back a local file. Browser console error: "Content-Security-Policy: The page's settings blocked the loading of a resource (media-src) at blob:null/<uuid> because it violates the following directive: 'default-src none'".

buildCSP() in internal/render/render.go emits script-src/style-src/img-src/font-src/connect-src but no media-src directive. Any <video>/<audio> element (or other media-consuming element) whose src is a blob: URL therefore falls through to default-src 'none' and is blocked, even though the blob was created client-side inside the sandboxed iframe (URL.createObjectURL on a File the user picked via <input type=file> or drag-and-drop) and never touches the network.

This is the same 'inlined/zero-egress asset' category already exempted for img-src data: and font-src data: (see the buildCSP doc comment's 'it's just a file' reasoning) — a local blob: object has no network egress either, so gating it behind the allowlist buys no security and breaks a canonical local-file-playback pattern (video/audio players, waveform tools, image/video editors that preview an imported file).

## Acceptance Criteria

buildCSP() adds a media-src directive permitting blob: (both the empty-allowlist and populated-allowlist branches) so <video>/<audio src=blob:...> renders without needing the origin allowlist. Existing render_test.go CSP tests still pass; add a test asserting media-src blob: is present in both branches. docs (technical_stack.md §4 / architecture.md render surface section) updated to mention media-src blob: alongside the existing img-src/font-src data: exemption if those docs enumerate directives.

