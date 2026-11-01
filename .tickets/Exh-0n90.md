---
id: Exh-0n90
status: closed
deps: []
links: []
created: 2026-11-01T21:40:15Z
type: task
priority: 2
assignee: mobile-tabsize-subagent
parent: Exh-hlio
tags: [mobile, ui, accessibility]
---
# Reduce mobile library tab bar bulk

Bottom navigation measures 65px plus the device inset, visually heavy against the tiny header.

Reference: mobile UI audit of 2026-11-01; Apple HIG https://developer.apple.com/design/human-interface-guidelines/ (accessibility, typography, tab-bars, segmented-controls, layout). CSS px are web design targets, not fixed native Apple bar-height specifications. Current HIG: 44x44pt default iOS controls, 28x28pt minimum. Preserve Exhibit native ES modules, server-rendered htmx partials, Stimulus conventions and self-hosted Phosphor icons. Do not redesign the artifact pill: geometry, placement, controls, menu structure and drag behavior are deferred. Global palette improvements may naturally affect shared colors.

## Acceptance Criteria

Aim for 52-56px total bar content height excluding safe-area inset, retaining >=44px targets, icons and readable labels. Preserve safe-area padding, last-card/pagination visibility, selected/focus states, and desktop rail. Ensure enlarged labels can grow gracefully. Own only bottom-navigation sizing and associated clearance CSS/tests; navigation visibility and labels are a separate story.

Validate relevant behavior at 320, 375, 390 and 430 CSS px, light/dark, and desktop. Check actual target rectangles, no body horizontal overflow, long labels and enlarged text; preserve safe areas and zoom. Run meaningful existing/targeted tests and project-required checks. Obtain independent Standards and Spec subagent reviews against a pinned base and this ticket, fix important findings, and document evidence/limitations in the PR. Never merge or deploy. Do not claim physical iPhone/VoiceOver testing without performing it.


## Notes

**2026-11-01T22:26:23Z**

Implemented compact 54px mobile rail with 45px link targets and intrinsic bottom-sticky flow clearance. Existing filter/navigation/race/no-JS/desktop/focus suites passed; 16 sizing cases passed at320/375/390/430 light/dark including a temporary single-label navigation fixture at200% text, simulated34px inset, pagination and short pages. Independent Standards/Spec reviews completed; two P2 test-harness issues fixed and verified at8d510bb4. Screenshots in docs/screenshots/Exh-0n90. Actual browser zoom, physical iPhone and VoiceOver untested. Full aggregate checks reserved for parent per task instructions. No deployment or merge.
