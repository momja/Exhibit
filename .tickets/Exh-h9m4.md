---
id: Exh-h9m4
status: closed
deps: []
links: []
created: 2026-11-01T21:40:15Z
type: task
priority: 1
assignee: mobile-readability-subagent
parent: Exh-hlio
tags: [mobile, ui, accessibility]
---
# Improve mobile text legibility and light-mode contrast

Gallery titles and route descriptions use 12.5px; fixed 26px metadata rows limit text growth. Light muted #888 has contrast 3.54:1 on white and 3.11:1 on #f0f0f0; labels #7b828e are 3.87:1 on white.

Reference: mobile UI audit of 2026-11-01; Apple HIG https://developer.apple.com/design/human-interface-guidelines/ (accessibility, typography, tab-bars, segmented-controls, layout). CSS px are web design targets, not fixed native Apple bar-height specifications. Current HIG: 44x44pt default iOS controls, 28x28pt minimum. Preserve Exhibit native ES modules, server-rendered htmx partials, Stimulus conventions and self-hosted Phosphor icons. Do not redesign the artifact pill: geometry, placement, controls, menu structure and drag behavior are deferred. Global palette improvements may naturally affect shared colors.

## Acceptance Criteria

Normal small text reaches >=4.5:1 on intended surfaces in both themes. Use roughly 16-17px primary mobile text and 13-14px secondary text where appropriate, with relative sizing and wrapping of important titles. Verify 200% text enlargement with no lost controls/content; retain 16px touch field protection and zoom. Preserve desktop density and do not change bar or tap-target sizing. Own palette/typographic tokens and scoped text styles. Coordinate card text wrapping with the target-sizing story; creation layout is separate.

Validate relevant behavior at 320, 375, 390 and 430 CSS px, light/dark, and desktop. Check actual target rectangles, no body horizontal overflow, long labels and enlarged text; preserve safe areas and zoom. Run meaningful existing/targeted tests and project-required checks. Obtain independent Standards and Spec subagent reviews against a pinned base and this ticket, fix important findings, and document evidence/limitations in the PR. Never merge or deploy. Do not claim physical iPhone/VoiceOver testing without performing it.


## Notes

**2026-11-01T22:24:11Z**

Implemented palette contrast, relative scoped mobile prose, full card-title wrapping and growing metadata rows. Independent Standards review: zero findings. Spec review found two P2 enlarged-field misses; composer line spacing/shrinking and sharing-heading wrapping fixed in f50e0f30 and independently verified. Eight focused Chromium suites passed, including both themes at 320/375/390/430 and desktop plus 200% sizing; six Playwright journeys passed. Visual matrix64 states: no horizontal page/dialog overflow. See docs/mobile-readability.md and screenshots for evidence/limits. Preserve targets title min-width/min-height44 when combining; creation route typography and agent composer CSS overlap documented. No physical iPhone/VoiceOver claim, no merge/deploy.
