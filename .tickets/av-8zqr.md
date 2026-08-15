---
id: av-8zqr
status: closed
deps: []
links: [av-s9ti]
created: 2026-08-14T15:40:00Z
type: task
priority: 2
assignee: Max Omdal
parent: av-cjkw
tags: [ui, mobile, pwa, frontend, a11y]
---
# Disable pinch-to-zoom only when launched from the home screen

Re-scoped return of [[av-s9ti]], which shipped and was then reverted in PR #89 review. The revert's objection was that a static `maximum-scale=1,user-scalable=no` in the viewport meta applies to *every* visit — including an ordinary Safari tab, where pinch-zoom is a page-level accessibility affordance (WCAG 1.4.4, Resize Text) — and that iOS 10+ ignores the directive there anyway, so it cost accessibility without buying the behavior.

This ticket narrows the ask to the case that actually motivated it: the app *saved to the home screen*. In a browser tab nothing changes and pinch-zoom is untouched; only a standalone launch — where the shell reads as an app, a pinch is usually a stray second finger on a scrolling grid, and there is no visible browser chrome to make the zoomed state obvious or easy to undo — turns it off.

Implementation is a small head-loaded script (`web/gallery/pwa.js`, served from the app origin) rather than a markup change, because the condition is a runtime one:

- gate on `navigator.standalone === true` (iOS home-screen launch) or `(display-mode: standalone|fullscreen)`; return immediately otherwise;
- rewrite the viewport meta to `maximum-scale=1,user-scalable=no` — the mechanism Chrome honours in an installed PWA on Android;
- cancel `gesturestart`/`gesturechange`/`gestureend`, which is what actually stops the pinch on WebKit, where the meta remains advisory. This is the half av-s9ti was missing, and why the original never worked on iOS.

Loaded via the shared `pwaHead` partial so it lands on every app-origin page in one place, and rendered blocking rather than deferred: it decides the page's zoom behaviour, so it must settle before first paint. The agent page (`agent.tmpl`) picks up `pwaHead` here — it was the one app-origin page av-fdcx missed, and a standalone app that stops behaving like an app on one route is worse than not doing this at all.

The render surface stays out of it, unchanged from av-s9ti's reasoning: artifacts are visitor-authored files and own their own viewport (architecture.md §1).

## Text-size control (PR #99 review)

Review objected that removing the pinch without an alternative fails WCAG 1.4.4 — the same objection that reverted av-s9ti, and a correct one. So the guard ships with a text-size control in the app-origin header (`textScale` partial): two icon buttons stepping 100 / 125 / 150 / 175 / 200%, hidden in the markup and revealed only by the standalone guard, with the choice persisted in app-origin `localStorage`.

It drives the same viewport-meta scale the guard already owns, and two measurements decided that:

- **CSS `zoom` was rejected.** At `zoom: 2` on a 390px viewport the document grows to 791px inside a 390px screen — media queries never see a narrower viewport, so it magnifies without reflowing. That is horizontal scrolling at large text (WCAG 1.4.10) as the price of satisfying 1.4.4. The meta scale reflows instead: the layout viewport becomes 195px and the breakpoints re-evaluate.
- **The scale must be present at parse time.** Rewriting the meta afterwards updates the attribute and changes nothing on screen. So a change is stored and applied by reloading — which is why `edit`, `new`, and `agent` carry `data-scale-reload-warn` and get a confirm first: they can hold an editor buffer, a pasted body, or a conversation, and nothing in this app tracks dirty state.

Measured on a 390×844 mobile viewport in a standalone window: stored 1 / 1.25 / 1.5 / 1.75 / 2 produce layout viewports of 390 / 312 / 260 / 223 / 195 and a matching `visualViewport.scale`, so 200% text is real. The header needed the control's space — it wraps and tightens below 640px only when the control is visible (`:has()`), so a browser tab renders exactly as before, and no width overflows that did not overflow already.

**Residual limits, recorded deliberately:**

- At 200% the layout viewport is 195px and the gallery pans horizontally. That floor is pre-existing — with the control hidden the page already stops reflowing at ~284px — so the control exposes it rather than causing it. Making the shell reflow that far is a responsive-design job, not this ticket's.
- Verified in Chromium under mobile emulation only. iOS is the primary target and WebKit is the engine the gesture half exists for, so AC5's on-device check is what actually closes this.

## Acceptance Criteria

1. Every app-origin page (gallery, new, detail, edit, notfound, agent) loads the guard, after its own viewport meta. 2. The guard's effects — the viewport scale, the cancelled gesture events, and revealing the control — sit behind the standalone gate, so a normal browser tab still pinch-zooms and shows no control. 3. The render surface's artifact document is untouched — no viewport rewrite, no guard script. 4. The control reaches 200%, persists across launches, and confirms before reloading on pages that can hold unsaved work. 5. Manual check on a real home-screen launch: pinch does not zoom, the control does resize text to 200%, and the same page in a Safari tab still pinch-zooms and shows no control.
