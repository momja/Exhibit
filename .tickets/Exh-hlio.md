---
id: Exh-hlio
status: in_progress
deps: []
links: []
created: 2026-11-01T21:40:15Z
type: epic
priority: 1
assignee: Max Omdal
tags: [mobile, ui, accessibility]
---
# Mobile interface ergonomics and accessibility

Implement the six accepted non-pill findings from the mobile interface audit in separate reviewable PRs. Retain a seventh story for artifact-pill design exploration, unassigned and unimplemented until the user decides. Reference: mobile UI audit of 2026-11-01; Apple HIG https://developer.apple.com/design/human-interface-guidelines/ (accessibility, typography, tab-bars, segmented-controls, layout). CSS px are web design targets, not fixed native Apple bar-height specifications. Current HIG: 44x44pt default iOS controls, 28x28pt minimum. Preserve Exhibit native ES modules, server-rendered htmx partials, Stimulus conventions and self-hosted Phosphor icons. Do not redesign the artifact pill: geometry, placement, controls, menu structure and drag behavior are deferred. Global palette improvements may naturally affect shared colors.

## Acceptance Criteria

Six implemented stories have individual PRs, independent review and important misses fixed. A separate holistic subagent reviews the combined patches for CSS collisions, responsive regressions and inconsistent behavior; important findings are fixed before handoff. Pill story stays open and deferred. No production deployment or merges.


## Notes

**2026-11-01T21:40:15Z**

Implementation stories are assigned to separate subagents. Pill story Exh-w6o4 is deliberately open, unassigned and excluded from implementation. Independent per-story reviews precede a combined-overlap review. Ticket-only commits on local main are required by TICKETS.md; code branches start from current origin/main.
