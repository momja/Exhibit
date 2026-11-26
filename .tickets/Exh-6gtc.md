---
id: Exh-6gtc
status: open
deps: [Exh-bjqg, Exh-b0b2]
links: []
created: 2026-11-26T05:50:23Z
type: task
priority: 2
assignee: Max Omdal
parent: Exh-i8q4
tags: [ui, detail]
---
# Artifact bar: band and overlay sizing from a ResizeObserver

Band mode reserves space below the frame (body.pip-inset .panels bottom); overlay on frame announcement sends heights as __avBarHeight; band sends 0. Band size and --exhibit-bar come from a ResizeObserver on the bar, bar sized in rem with min-height (docs/artifact-bar.md §3, §6.4). Short viewports (<450px) start compact.

## Acceptance Criteria

Chromium test: band resizes the frame, overlay variable reaches the frame; 200% root font size band matches bar's measured height in both states.

