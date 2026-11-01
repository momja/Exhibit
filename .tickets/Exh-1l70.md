---
id: Exh-1l70
status: in_progress
deps: []
links: []
created: 2026-11-01T21:40:15Z
type: task
priority: 2
assignee: mobile-navigation-subagent
parent: Exh-hlio
tags: [mobile, ui, accessibility]
---
# Keep library destinations visible and clearly labeled

The library navigation disappears when both sharing counts are zero. Short labels With me / By me lose context.

Reference: mobile UI audit of 2026-11-01; Apple HIG https://developer.apple.com/design/human-interface-guidelines/ (accessibility, typography, tab-bars, segmented-controls, layout). CSS px are web design targets, not fixed native Apple bar-height specifications. Current HIG: 44x44pt default iOS controls, 28x28pt minimum. Preserve Exhibit native ES modules, server-rendered htmx partials, Stimulus conventions and self-hosted Phosphor icons. Do not redesign the artifact pill: geometry, placement, controls, menu structure and drag behavior are deferred. Global palette improvements may naturally affect shared colors.

## Acceptance Criteria

Signed-in library spaces remain available through zero/nonzero sharing counts with useful empty states, while respecting anonymous/read-only access. Use Library, Shared with me, Shared by me where feasible. Preserve active states, search/filter/navigation context and desktop rail. Add meaningful tests for empty sharing states and transitions. Own gallery space data/templates and visibility selectors; leave tab geometry to the tab-sizing story.

Validate relevant behavior at 320, 375, 390 and 430 CSS px, light/dark, and desktop. Check actual target rectangles, no body horizontal overflow, long labels and enlarged text; preserve safe areas and zoom. Run meaningful existing/targeted tests and project-required checks. Obtain independent Standards and Spec subagent reviews against a pinned base and this ticket, fix important findings, and document evidence/limitations in the PR. Never merge or deploy. Do not claim physical iPhone/VoiceOver testing without performing it.

