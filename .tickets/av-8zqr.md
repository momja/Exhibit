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

**Residual accessibility trade-off, recorded deliberately:** inside the installed app a low-vision user loses pinch-zoom, and WCAG 1.4.4 does not exempt standalone display mode. The tab remains fully zoomable and is the escape hatch; if the app shell later grows its own text-scaling control, that is the right place to retire this note.

## Acceptance Criteria

1. Every app-origin page (gallery, new, detail, edit, notfound, agent) loads the guard, after its own viewport meta. 2. The guard's effects — the viewport rewrite and the cancelled gesture events — sit behind the standalone gate, so a normal browser tab still pinch-zooms. 3. The render surface's artifact document is untouched — no viewport rewrite, no guard script. 4. Manual check: pinch on the gallery grid from a home-screen launch does not zoom; the same page in a Safari tab does.
