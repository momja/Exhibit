---
id: Exh-2xch
status: in_progress
deps: []
links: []
created: 2026-11-01T21:40:15Z
type: task
priority: 2
assignee: mobile-creation-subagent
parent: Exh-hlio
tags: [mobile, ui, accessibility]
---
# Compact the mobile creation-method chooser

At 390x844, the three creation method cards consume 438px; form starts near y601 and Start building near y918.

Reference: mobile UI audit of 2026-11-01; Apple HIG https://developer.apple.com/design/human-interface-guidelines/ (accessibility, typography, tab-bars, segmented-controls, layout). CSS px are web design targets, not fixed native Apple bar-height specifications. Current HIG: 44x44pt default iOS controls, 28x28pt minimum. Preserve Exhibit native ES modules, server-rendered htmx partials, Stimulus conventions and self-hosted Phosphor icons. Do not redesign the artifact pill: geometry, placement, controls, menu structure and drag behavior are deferred. Global palette improvements may naturally affect shared colors.

## Acceptance Criteria

On phones, present compact Agent / Paste / URL choices with one clear explanation for the selected method; put the active form immediately below. Preserve all three flows, validation, entered values when switching, default agent behavior, server access rules and desktop usability. Use appropriate accessible semantics/focus/keyboard operation. Each choice has >=44px touch target; first required field and primary action should be easy to reach without scrolling past large inactive explanations. Own new page templates, new.css/new.js and targeted tests.

Validate relevant behavior at 320, 375, 390 and 430 CSS px, light/dark, and desktop. Check actual target rectangles, no body horizontal overflow, long labels and enlarged text; preserve safe areas and zoom. Run meaningful existing/targeted tests and project-required checks. Obtain independent Standards and Spec subagent reviews against a pinned base and this ticket, fix important findings, and document evidence/limitations in the PR. Never merge or deploy. Do not claim physical iPhone/VoiceOver testing without performing it.

