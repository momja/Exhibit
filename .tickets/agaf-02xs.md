---
id: agaf-02xs
status: closed
deps: [av-ghvs]
links: [av-dwe2]
created: 2026-08-13T18:31:03Z
type: bug
priority: 1
assignee: Max Omdal
tags: [render, gallery, memory, safari]
---
# Detail page embeds the full artifact source — Safari stalls on the multi-MB page; sandboxed data: fetches refused

Two Safari/Chromium failures behind one artifact class (multi-MB bodies, e.g. snapshots with vendored wasm — pokeemerald-wasm at 16 MB):

1. The detail page rendered the artifact's full source in a `<pre>` beside the iframe, making the page itself as large as the artifact (16.7 MB). Safari stalls on a response that size: the navigation never completes and the artifact "never loads" in the framed detail page (top-level works). Chromium survives it, but under real memory pressure the same weight amplifies into a multi-GB renderer runaway.
2. WebKit refuses large `fetch()` of `data:` URLs from an opaque-origin sandbox, so the artifact's vendored wasm (12 MB `data:` URI) never booted in Safari's iframe even when the page did load.

Shipped fix:
- The detail page never embeds the source — the body lives on the edit page (CodeMirror); page weight is now independent of artifact size.
- The render preamble gains a `data:` fetch **compatibility shim** answering `data:` GETs from locally constructed Responses (grants nothing: the bytes are already in the document).
- The willReadFrequently/CSS canvas mitigation trialed for the leak half was removed as ineffective; the "leak" was concluded to be pressure amplification of the heavy page, not a per-frame leak. The notes below tell the full story.

## Acceptance Criteria

- Detail-page weight is independent of artifact size: the handler never reads the source blob, the page carries no `<pre>`/source controls (pinned by `TestDetailPageDoesNotEmbedSource`); measured 5.7 KB for the 16 MB snapshot.
- Safari: the artifact boots in the sandboxed detail-page iframe — vendored `data:` assets load through the preamble's compatibility shim; Chromium unaffected.
- The Chromium runaway is not reproducible after the source removal; no per-frame canvas leak is claimed.
- The removed canvas mitigation stays out of the preamble (pinned by render tests).
- The preamble's fetch wrapper installs before the vendorer's — the two only work composed (pinned by `TestPreambleFetchWrapperPrecedesArtifactScripts`).


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

**2026-08-15T20:36:08Z**

Ticket top rewritten (user-approved) to match what shipped: title/description/acceptance criteria now describe the source-panel removal, the data: fetch compatibility shim, and the invariant that detail-page weight is independent of artifact size. The abandoned canvas-leak mitigation stays in the notes above as the investigation record; the branch name is historical and deliberately not renamed.
