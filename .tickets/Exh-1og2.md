---
id: Exh-1og2
status: in_progress
deps: []
links: []
created: 2026-11-01T21:40:15Z
type: task
priority: 1
assignee: mobile-targets-subagent
parent: Exh-hlio
tags: [mobile, ui, accessibility]
---
# Enlarge mobile headers and frequent touch targets

Header is 33px on phones; its controls are 32px. Card overflow is 22x20px and sharing trigger about 25x19px. Edit back link can be only 16.5px high.

Reference: mobile UI audit of 2026-11-01; Apple HIG https://developer.apple.com/design/human-interface-guidelines/ (accessibility, typography, tab-bars, segmented-controls, layout). CSS px are web design targets, not fixed native Apple bar-height specifications. Current HIG: 44x44pt default iOS controls, 28x28pt minimum. Preserve Exhibit native ES modules, server-rendered htmx partials, Stimulus conventions and self-hosted Phosphor icons. Do not redesign the artifact pill: geometry, placement, controls, menu structure and drag behavior are deferred. Global palette improvements may naturally affect shared colors.

## Acceptance Criteria

Mobile header has a 52-56px content band plus top inset; frequent header/search/back/card actions have non-overlapping >=44x44px hit areas. Icons remain visually proportionate. Card metadata row accommodates larger targets. Reorganize secondary header items if necessary so 320px search remains useful. Preserve desktop density and hosted/self-hosted account access. Do not alter bottom-tab styling, typography/color tokens or artifact-pill controls. Own sizing tokens, shared non-pill control/header CSS, gallery card sizing and necessary header templates.

Validate relevant behavior at 320, 375, 390 and 430 CSS px, light/dark, and desktop. Check actual target rectangles, no body horizontal overflow, long labels and enlarged text; preserve safe areas and zoom. Run meaningful existing/targeted tests and project-required checks. Obtain independent Standards and Spec subagent reviews against a pinned base and this ticket, fix important findings, and document evidence/limitations in the PR. Never merge or deploy. Do not claim physical iPhone/VoiceOver testing without performing it.

