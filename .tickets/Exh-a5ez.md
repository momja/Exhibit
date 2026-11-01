---
id: Exh-a5ez
status: in_progress
deps: []
links: []
created: 2026-11-01T21:40:15Z
type: task
priority: 2
assignee: mobile-agenttabs-subagent
parent: Exh-hlio
tags: [mobile, ui, accessibility]
---
# Simplify mobile Chat and Preview segmented controls

Chat/Preview row measures about 60.5px but the segments are only 35.5px high; nested padding adds visual bulk.

Reference: mobile UI audit of 2026-11-01; Apple HIG https://developer.apple.com/design/human-interface-guidelines/ (accessibility, typography, tab-bars, segmented-controls, layout). CSS px are web design targets, not fixed native Apple bar-height specifications. Current HIG: 44x44pt default iOS controls, 28x28pt minimum. Preserve Exhibit native ES modules, server-rendered htmx partials, Stimulus conventions and self-hosted Phosphor icons. Do not redesign the artifact pill: geometry, placement, controls, menu structure and drag behavior are deferred. Global palette improvements may naturally affect shared colors.

## Acceptance Criteria

Provide a compact segmented row with >=44px targets and less surrounding chrome. Keep Chat/Preview as related task views, not app-level tabs. Preserve selected/update states, pane content and scroll position, preview actions and desktop side-by-side layout. Implement complete keyboard/focus semantics for the chosen tab pattern and hide decorative glyphs from accessible names. Check short viewports and composer reachability. Own pane-switch CSS, tab markup, showPane behavior and relevant tests; do not redesign composer or artifact pill.

Validate relevant behavior at 320, 375, 390 and 430 CSS px, light/dark, and desktop. Check actual target rectangles, no body horizontal overflow, long labels and enlarged text; preserve safe areas and zoom. Run meaningful existing/targeted tests and project-required checks. Obtain independent Standards and Spec subagent reviews against a pinned base and this ticket, fix important findings, and document evidence/limitations in the PR. Never merge or deploy. Do not claim physical iPhone/VoiceOver testing without performing it.

