---
id: Exh-b0b2
status: open
deps: []
links: []
created: 2026-11-26T05:50:23Z
type: task
priority: 2
assignee: Max Omdal
parent: Exh-i8q4
tags: [render, ui]
---
# Artifact bar: render preamble bar script and protocol

Replace the prototype scrollReporterScript with barScript in internal/render/render.go implementing docs/artifact-bar.md §4: {__avBar:'scroll',dir}, {__avBar:'use'}, {__avBar:'mode',mode} to API_ORIGIN; {__avBarHeight:n} receiver (origin API_ORIGIN, source window.parent, finite [0,400]) setting --exhibit-bar; overlay mode sets scroll-padding-bottom (§6.2). Injected under !noHost && !widget.

## Acceptance Criteria

render_test.go: present for framed artifact render; absent for widgets, /s/:id and top-level; height receiver rejects non-numbers, negatives, >400; mode reader requires exact 'overlay'; names exhibit-bar, overlay, --exhibit-bar asserted.

