---
id: Exh-eeld
status: closed
deps: [Exh-bjqg]
links: []
created: 2026-11-26T05:50:23Z
type: task
priority: 2
assignee: Max Omdal
parent: Exh-i8q4
tags: [ui, accessibility]
---
# Artifact bar: accessibility requirements (doc §6)

Implement docs/artifact-bar.md §6: click intercept only after a pointerdown on the bar; compacted controls stay focusable and focus expands; overlay scroll-padding; overlay contrast (solid/≥95% background); reduced motion keeps band at full height; resolve role=toolbar (roving focus or labelled group); hover 500ms grace; target sizes.

## Acceptance Criteria

Tests per §7 Accessibility: focus-less click on compact title expands and opens the card; Tab into compact bar expands; pointer tap on strip only expands; overlay keyboard-focused bottom control scrolls clear; 200% font band matches.

