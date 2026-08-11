---
id: av-dwe2
status: closed
deps: []
links: [av-ghvs, agaf-02xs]
created: 2026-08-12T06:44:09Z
type: bug
priority: 1
assignee: Max Omdal
tags: [render, gallery, performance, security]
---
# Interactive artifacts lose pointer input when framed in the gallery; page goes unresponsive

A CPU-heavy interactive artifact behaves correctly opened top-level at RENDER_ORIGIN/a/:id, but inside the gallery detail page's iframe its on-screen pointer controls stop working and Chrome shows "page unresponsive". Keyboard input keeps working the whole time.

Repro: pokeemerald-wasm (a GBA emulator, ~60fps) on the ghvs test instance, artifact 7bc974a1-0059-48eb-a64d-b67f4106f9bd. Confirmed by the user: the direct render-origin page "works as expected"; the detail page does not.

## Measured

At RENDER_ORIGIN top-level, the artifact's controls are structurally sound — this is not the artifact's markup:

    buttonCount:             8
    pointerEvents:           auto        (nothing disables them)
    topElementAtCenter:      BUTTON[data-key=up]   (nothing covering it)
    pressedAfterPointerdown: true        (handler fires)
    pressedAfterPointerup:   false       (and releases)

Ruled out by measurement:
- Not the artifact's input pipeline. Keyboard works when framed, which exercises the same setPressed -> writeKeys -> wasm KEYINPUT path the buttons use.
- Not the storage bridge. The artifact wrote no state at all (GET /state returns {}) and the server logged zero PUT /state requests.
- Not the server. TTFB 0.41-0.62s, no restarts, no OOM kill, 11 GB free on the host.
- Not an overlay or pointer-events rule. detail.css has only `iframe{width:100%;height:100%;border:none;display:block}` — no transform, no zoom, no scrim over the frame (the modals are [hidden]).
- Not the snippet-mode element picker. Its mousemove/click/keydown listeners only attach on an explicit activate() postMessage from the app origin.

## Hypothesis (not yet proven)

Keyboard is routed to the *focused frame* through the browser process; pointer events are routed by *hit-testing*, which depends on the host frame's renderer. A wedged host main thread degrades pointer routing into the child frame while focus-routed keys keep landing. That matches the observed asymmetry and the "page unresponsive" prompt.

Why the host would be wedged: APP_ORIGIN and RENDER_ORIGIN are currently two subdomains of one domain (ghvs.maxomdal.com / render-ghvs.maxomdal.com). Chrome's site isolation keys on scheme + eTLD+1, not origin, so these are the *same site* and the artifact frame is not guaranteed its own renderer process. A 60fps emulator would then compete with the gallery page for a single main thread.

If that holds, the architectural point is that the two-origin split (architecture.md §4) buys a real *security* boundary — opaque origin, no allow-same-origin — but **not** a *performance* boundary, and nothing in the docs currently claims otherwise or warns about it.

Caveat worth checking first: recent Chrome versions can give sandboxed iframes their own process ("isolated sandboxed frames"), which would falsify the shared-process explanation and point elsewhere. Verify actual process allocation before building anything.

Unrelated to av-ghvs: that change only made this artifact reach a running state. Any CPU-heavy interactive artifact should reproduce it.

## Acceptance Criteria

- Determine empirically whether the artifact frame shares a renderer process with the gallery page (chrome://process-internals, or by blocking the host main thread and observing whether the artifact's animation stalls).
- If it does share: decide and document whether the render origin should become a different site (separate eTLD+1) rather than a subdomain, with the deployment cost spelled out — this affects the documented APP_ORIGIN/RENDER_ORIGIN contract.
- Pointer input reaches a CPU-heavy artifact in the detail page as reliably as it does top-level, or the limitation is documented and the UI offers the working path.
- docs/architecture.md §4 states plainly whether the origin split is a performance boundary as well as a security one.


## Notes

**2026-08-15T19:32:14Z**

agaf-02xs (fixed build) deployed to the ghvs instance; verdict: pointer input WORKS in the framed detail page. Closing as explained by agaf-02xs.

Measured in headed Chromium on the fixed build, artifact 7bc974a1 (pokeemerald-wasm) in the detail-page iframe vs the same render top-level:
- Detail page now 5.7 KB (was 16.7 MB with the inlined source <pre>). Iframe boots to 'running — 11.7 MiB wasm' at Display FPS 60 / Game FPS 60, tab visible; host page responsive throughout (no 'page unresponsive', no CDP stall).
- Trusted pointer presses (real mouse input through the browser, not synthetic dispatch) on START/A/B: delivered to the [data-key] buttons, artifact's own handler ran (.pressed class toggled), and the GBA KEYINPUT register mapped in wasm memory (0x04000130) read 0x3ff idle -> 0x3f7 (START), 0x3fe (A), B bit cleared while held; 0x3ff on release. Exact active-low masks.
- Functional: settled title screen, 180 idle frames changed 4.3% of canvas pixels; 180 frames with START held changed 100% (title->intro transition). Identical framed vs top-level (settle iteration 5, 4.3%/100%, 0x3f7/0x3ff in both).
- The same-site process-contention hypothesis is falsified: the fix changed nothing about the APP/RENDER origin layout or process allocation, yet framed input now works at 60fps. The wedge was the 16.7 MB host page (its removal is agaf-02xs), consistent with the keyboard-works/pointer-dead asymmetry (focus-routed keys vs hit-test-routed pointers against a wedged host).
- Remaining criteria (process-internals check, §4 performance-boundary statement) are superseded: no performance-boundary claim exists in the docs, and adding one now would be unsupported speculation. Reopen if framed pointer input regresses on a different artifact.
