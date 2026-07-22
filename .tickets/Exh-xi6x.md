---
id: Exh-xi6x
status: in_progress
deps: []
links: []
created: 2026-07-22T00:40:51Z
type: bug
priority: 2
assignee: Max Omdal
tags: [security, csp, render]
---
# Sandboxed artifact CSP blocks blob: worker scripts (script-src has no blob:)

buildCSP (internal/render/render.go) generates script-src without a blob: source. An artifact that spawns a Worker from a blob: URL — the standard workaround for running a cross-origin worker script (e.g. ffmpeg.wasm's worker.js from unpkg) inside our opaque-origin sandboxed iframe (sandbox=allow-scripts, no allow-same-origin) — gets blocked: 'The page's settings blocked a worker script (worker-src) ... because it violates script-src'. There is no worker-src directive, so the browser falls back to script-src, and script-src only lists 'unsafe-inline' 'unsafe-eval' plus allowlisted origins — no blob:. This is the same class of gap already fixed for media-src (blob: video/audio playback, per the existing TestBuildCSPMediaSrcAlwaysAllowsBlob test).

## Acceptance Criteria

script-src permits blob: in both the empty-allowlist and populated-allowlist branches of buildCSP, so a Worker constructed from a blob: URL executes. Reasoning: script-src already carries 'unsafe-eval', so permitting blob: scripts grants no capability beyond what eval already allows — it's a zero-egress exemption like the existing media-src blob: and img-src/font-src data: carve-outs.

