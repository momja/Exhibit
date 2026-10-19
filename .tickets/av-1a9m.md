---
id: av-1a9m
status: closed
deps: [av-kyhl]
links: []
created: 2026-08-01T18:50:13Z
type: feature
priority: 3
assignee: Max Omdal
tags: [ui, gallery, performance]
---
# Bound the number of live widget frames on the gallery

Each card with a widget (av-fafu) loads its own document from the render origin, and every one of them is a live page running its own JS for as long as it is in the DOM. The cost is linear in library size with no ceiling.

Measured on a 60-card library, all sixty carrying the run-tracker sample widget:
  - server side is not the problem: 60 widget documents render in 0.15s total, 6-way parallel, ~13 KB each
  - client side: 626 ms to load event, 26 MB JS heap, mean widget fetch 312 ms

That is with a small widget that renders once and stops. A widget that animates, polls, or holds a ResizeObserver (the run-tracker sample already holds one) keeps costing after it is offscreen, and nothing caps how many run at once. loading='lazy' is the only mitigation today and it stops helping the moment the user scrolls.

Deliberately NOT doing this yet: it adds complexity to the render path, and the sensible budget depends on having a window/page notion to hang it on.

## Design

Options, to be chosen when this is picked up:
  - Freeze offscreen tiles: an IntersectionObserver blanks/detaches the frame's src when the card leaves the viewport by some margin, restoring it on re-entry. Keeps the budget bounded regardless of library size; costs a reload per re-entry (cheap — 13 KB, no-store, already re-rendered per request).
  - Cap live tiles at N most-recent and render the default tile beyond it. Simpler, but makes the tile inconsistent for reasons the user cannot see.
  - Do nothing beyond paging, if infinite scroll keeps the resident set small enough in practice — worth measuring before building anything.
Measure first with the numbers above as the baseline.

## Acceptance Criteria

Widget frame count (and JS heap) stays bounded as the library grows, measured against the 60-card baseline in the description.


## Notes

**2026-10-19T06:25:50Z**

Implemented viewport-buffered widget unloading in a gallery-local native module. Widgets beyond a 400px buffer lose their browsing context while their server-created iframe node and monogram remain available. Nearby widgets survive paging; re-entry reloads through an owner-scoped, no-store app redirect that issues a fresh render token. Search releases removed cards and observers. Widget health work is disposed on unloading, and early readiness reports cannot restart later deadlines.

Measured with stock run-tracker widgets at 1280x800: 60/250 artifacts previously retained 60/250 frames after traversal; bounded runs peaked at 45/50 settled frames and retained 35 after returning to the top. At 375x812, 250 cards peaked at 10 and retained 8. At 3840x2160, 216 cards loaded automatically with all 208 visible widgets present and healthy. The preview clamps JS heap figures to 10,000,000 bytes, so exact heap savings could not be measured or claimed.

Validation: make assets; go test -timeout 20m ./...; focused checks after merging current origin/main; native module/health regression tests; real-browser paging, re-entry with fresh credentials, unchanged nearby frame identity, destroyed offscreen browsing contexts, and search cleanup. Measurements and method are in docs/widgets.md, with desktop/mobile screenshots in docs/screenshots/av-1a9m/.
