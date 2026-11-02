---
id: Exh-2xch
status: closed
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


## Notes

**2026-11-01T22:11:24Z**

Implemented compact phone Agent / Paste / URL native choices, one selected explanation, 44px targets/actions, wrapping and bottom safe-area clearance. Desktop cards and all form/editor flows remain intact. At 390px, the chooser shrank from 438px to 44px; the form moved from y601 to y245 and the primary action from y918 to y581. Evidence: docs/screenshots/Exh-2xch.

Implementation reviewed at 82defc9efaf19d55cfac70b4a7c8471a561b5702. Independent Standards and Spec reviews found no standards/spec misses. A P2 browser-test navigation race was fixed and independently verified, with no remaining findings.

The full Go/Node/Chromium suite, required browser gate, and six Playwright journeys passed. Diff-filtered lint has zero new issues; full lint reports 24 existing issues in 16 files unchanged from the base. The 320/375/390/430/1280px light/dark matrix, long labels and doubled computed text, simulated 34px safe area, keyboard operation, and field/editor retention checks passed. No physical iPhone, VoiceOver, real zoom, or OS keyboard checks were performed. No pill/global token, merge, or deployment changes.
