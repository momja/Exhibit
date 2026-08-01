---
id: av-1a9m
status: open
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

