---
id: Exh-w6o4
status: open
deps: []
links: []
created: 2026-11-01T21:40:15Z
type: task
priority: 2
assignee: Max Omdal
parent: Exh-hlio
tags: [mobile, ui, accessibility]
---
# Explore artifact pill ergonomics without consuming micro-app space

DEFERRED BY USER. Artifacts are micro-apps with their own controls; Exhibit must keep its controls available while occupying minimal space. Do not implement the audit recommendation for a permanent dock.

Reference: mobile UI audit of 2026-11-01; Apple HIG https://developer.apple.com/design/human-interface-guidelines/ (accessibility, typography, tab-bars, segmented-controls, layout). CSS px are web design targets, not fixed native Apple bar-height specifications. Current HIG: 44x44pt default iOS controls, 28x28pt minimum. Preserve Exhibit native ES modules, server-rendered htmx partials, Stimulus conventions and self-hosted Phosphor icons. Do not redesign the artifact pill: geometry, placement, controls, menu structure and drag behavior are deferred. Global palette improvements may naturally affect shared colors.

## Acceptance Criteria

Remain open and unassigned. Discuss compact floating designs, direct Library vs a single Exhibit launcher, on-demand Share/Edit/Download/permissions, discoverable reposition/hide, safe-area/keyboard handling and recoverability when covering artifact controls. No design has been accepted. User input is required before assigning implementation or creating a pill implementation PR.

