---
id: agaf-02xs
status: in_progress
deps: [av-ghvs]
links: []
created: 2026-08-13T18:31:03Z
type: bug
priority: 1
assignee: Max Omdal
tags: [render, memory, chromium]
---
# Canvas putImageData memory leak in sandboxed artifact iframes (Chromium)

Artifacts that call ctx.putImageData() every frame (e.g. the pokeemerald-wasm game) leak ~500KB/frame in the iframe renderer's PartitionAlloc when embedded in Exhibit's sandboxed OOP iframe. Observed live: renderer grew 5.8GB->9.0GB at ~30MB/s at 60fps; JS heap flat at 54MB; 26,904 PartitionAlloc regions (tag 255) of ~128-384KB retained. Top-level renders and local sandboxed-iframe harnesses do NOT leak; only the real Exhibit context leaks. No-op'ing putImageData via CDP stopped growth instantly (flat at 1.69GB while game kept running). Fix approach: carry the mitigation in the render shim instead of the artifact - (1) fetch wrapper translating data: URL fetches into locally constructed Responses (fixes Safari never-loading, WebKit data: fetch flakiness in opaque sandbox), (2) HTMLCanvasElement.prototype.getContext wrapper forcing willReadFrequently:true on 2d contexts, (3) optional injected CSS with !important neutralizing image-rendering/border-radius on canvases, gated behind a per-artifact state flag so visuals of pixel-art artifacts are untouched by default.

## Acceptance Criteria

With the flag enabled on an artifact, renderer footprint stays flat (no sustained growth) while the artifact runs at 60fps in the Exhibit iframe. With the flag disabled, behavior unchanged. Safari: artifact loads in the sandboxed iframe via the data: fetch wrapper.


## Notes

**2026-08-13T22:01:56Z**

ATTEMPT 1 (deployed agaf-02xs) RESULT: FAILED. willReadFrequently:true + canvas CSS overrides (pixelated/border-radius !important) did NOT stop the leak. Verified the context attributes reported willReadFrequently:true and the game ran at 60fps, yet the iframe renderer grew 7.3GB->10GB (~23MB/s). Conclusion: the leak is not the accelerated-canvas path nor the mask/scaling path; it is tied to the per-frame canvas frame submission itself (putImageData no-op remains the only confirmed stopper). NOTE: harness (localhost, sandboxed OOP iframe, identical code) has never leaked — but all harness runs may have been throttled background tabs; the foreground-60fps harness control is still unrun (machine OOM'd before it). Next hypotheses: (1) wasm-backed ImageData source copy retention -> shim wraps putImageData to copy into a plain ImageData first; (2) Helium-specific (fork of Chromium 151) -> test stock Chrome; (3) cross-origin parent present path. The data: fetch wrapper (Safari fix) deployed successfully and is unaffected.

**2026-08-13T22:41:32Z**

Deploy 2 (agaf-02xs): Safari blocker found and fixed — the detail page inlined the FULL artifact source in a <pre> panel (16.7MB page for this artifact); Safari stalls on that response, so the artifact 'never loads' in Safari (Chromium handles it). Fixed: source panel is now lazy (GET /api/artifacts/{id}/source streams the body on demand; detail page is now 5.6KB / 0.1s). The render shim's data: fetch wrapper remains deployed. Chrome leak status: unchanged (willReadFrequently+CSS did NOT stop it; putImageData no-op remains the only confirmed stopper).

**2026-08-13T22:50:07Z**

Cleanup done: ineffective canvas mitigation removed from shim (commit 9ee0cca); branch redeployed to agaf-02xs (fetch wrapper present, mitigation gone); ticket committed to main (912d6dd); test servers/samplers killed; /tmp investigation files removed (kept tripwire + wasm for the open leak work). SAFARI PORTION: RESOLVED (lazy source panel + data: fetch shim, user-confirmed). CHROMIUM LEAK: still open — putImageData no-op remains the only confirmed stopper; next steps: stock-Chrome repro, plain-ImageData source test, foreground-60fps harness control.

**2026-08-13T23:09:58Z**

Chromium leak re-investigation (user reports no leak now): A/B harness tests on a HEALTHY machine — same sandboxed artifact iframe at 60fps with (a) tiny parent: flat 125MB; (b) hidden 16.7MB pre: flat; (c) visible 16.7MB pre panel: one-time ~700MB parse/raster spike then flat. NO continuous leak in any harness condition. Re-examining all leak observations: every continuous-growth session (user's 350MB/s storm, deployed-1 23MB/s) occurred with free RAM at ~0.1GB and heavy swap thrash; the boot churn (16MB fetch + 12MB compile + 256MB instantiate + per-frame putImageData presents) is large but normally recycled. CONCLUSION: the Chromium 'leak' was most likely a memory-pressure-driven amplification of the same heavy load the giant 16.7MB detail page created — same root aggravator as the Safari hang, different failure mode (Safari: deterministic load hang; Chromium: pressure-dependent runaway under swap). The lazy source panel removed the aggravator; leak not reproducible since. Not a deterministic per-frame leak on healthy hardware.

**2026-08-14T03:42:28Z**

PR opened: https://github.com/momja/Exhibit/pull/98 (branch bug/agaf-02xs/canvas-leak-mitigation, 3 commits).

**2026-08-15T16:43:57Z**

Correction: the detail-page source <pre> was never a feature (leftover from template extraction; mobile CSS hid it; edit page is the way to the code). Reworked the fix to pure removal — no lazy panel, no /source endpoint. Branch reworked to 3 clean commits (db86286 shim, 2ca6fa0 source removal, f744dd8 mitigation removal), force-pushed; PR #98 updated; test instance redeployed (detail page has no pre/source controls).

**2026-08-15T17:33:53Z**

Merged bug/av-ghvs/inline-runtime-fetched-assets into this branch (merge cddbffc; only conflict was the ticket file, resolved to the fix branch's version). agaf-02xs now DEPENDS on av-ghvs. Redeployed agaf-02xs and verified end-to-end: POST /api/artifacts {url: pokeemerald.com, snapshot:true} now produces a self-contained artifact (12MB wasm inlined as data: URI + injected fetch shim; render doc 16.4MB), and the artifact boots at 60fps in the sandbox (status: 'running — 11.7 MiB wasm'). Snapshot ingest no longer fails for runtime-fetched wasm.
