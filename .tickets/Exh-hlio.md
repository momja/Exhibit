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

## Review handoff

Six implementation stories are complete in separate PRs:

| Story | PR | Base |
| --- | --- | --- |
| Exh-1og2 — headers and touch targets | https://github.com/momja/Exhibit/pull/260 | Tracking PR #256 |
| Exh-0n90 — compact library tabs | https://github.com/momja/Exhibit/pull/262 | Tracking PR #256 |
| Exh-1l70 — stable full-label navigation | https://github.com/momja/Exhibit/pull/259 | Tab sizing PR #262 |
| Exh-h9m4 — readability and contrast | https://github.com/momja/Exhibit/pull/261 | Tracking PR #256 |
| Exh-2xch — compact creation chooser | https://github.com/momja/Exhibit/pull/258 | Tracking PR #256 |
| Exh-a5ez — Chat/Preview controls | https://github.com/momja/Exhibit/pull/263 | Tracking PR #256 |

Each story received independent Standards and Spec reviews and fixes. A separate holistic subagent reviewed all six together, with fresh reviewers verifying fixes. Important findings are addressed: actual height clearance for full/enlarged tab labels, readable title target composition, selected creation-explanation coverage, short-screen composer checks with coarse touch, resize focus timing, and mobile layout while the page module loads. Final review evidence: Final source `1841bae0` independently reviewed on Standards and Spec axes: no remaining important findings; the startup-layout P2 is resolved. One optional P3 observation notes duplicated startup/runtime CSS declarations; distinct lifecycle gates justify keeping these simple rules, so no refactor is required..

Combined source snapshot: https://github.com/momja/Exhibit/commit/1841bae01c8d3c55855264073f403793b72f7a36. This is a review branch, not a merge to main or deployment. Validation: `make build`; final Chromium/Node/preview suite at `1841bae0` — 211 tests/subtests passed, no failures or skips; required browser gate passed; all six Playwright journeys passed (15.6s); diff-filtered `golangci-lint` reported 0 issues. The full API suite at `79fe43ad` passed 1,357 tests/subtests with 3 optional S3 skips. All other Go packages passed in the preceding full run; its two API failures were corrected and covered by the subsequent API/browser runs. The final startup amendment is included in the final browser/Node/preview checks.

The pre-existing Details child-control focus edge during desktop-to-phone resize is recorded as a non-blocking follow-up. Chromium viewport, text, touch and inset simulations were used; physical iPhone Safari, VoiceOver, actual zoom/on-screen keyboard and live Clerk are not verified. Full lint retains 24 pre-existing findings; changed-code lint passes.

Exh-w6o4 remains open and unassigned. No artifact-pill design is selected or implemented. Its geometry, placement, controls, menu and dragging remain deferred; shared palette changes can affect its colors. The epic remains in progress for that design decision.

Combined layout samples captured during integration testing, before the final startup/focus timing fixes: [375px gallery](../docs/screenshots/Exh-hlio/gallery-375-light.png), [320px enlarged navigation](../docs/screenshots/Exh-hlio/navigation-320-dark-enlarged.png), [320px creation](../docs/screenshots/Exh-hlio/creation-320-light.png), [320px enlarged composer](../docs/screenshots/Exh-hlio/composer-320-enlarged.png). These are Chromium captures, with text enlargement and touch simulation where stated.
