---
id: Exh-a5ez
status: closed
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


## Notes

**2026-11-01T22:29:47Z**

Implemented 51px mobile pane row with >=44px targets, wrapping enlarged labels, roving keyboard tabs, mobile tabpanel/desktop region semantics and decorative glyph hiding. Same-pane Snippet actions retain focus. Hidden/inert overlapping mobile panels preserve iframe scroll (fresh response regression exposed display:none resetting120px to0). Focused rebuilt-asset tests passed19.941s: AgentPanesBrowser, GalleryPageScriptSuite, AgentPreview*, DirectOpenPreviewSwapBrowser;320/375/390/430 light/dark, large labels,568px viewport, desktop, draft/scroll/state/htmx and keyboard. Independent Standards+Spec reviews and two fix reviews completed; no remaining findings at implementation7e26f5b24e259f672f7184dfc7fefc11b45f712d. Evidence docs/screenshots/Exh-a5ez. Full aggregate suite delegated to parent; no physical iPhone/VoiceOver testing. No merge or deployment.

**2026-11-01T23:09:05Z**

Aggregate follow-up: fixed a deterministic breakpoint focus race by synchronizing pane/tab-strip/divider visibility with captured focus, roles and inertness. A delayed matchMedia callback regression covers both resize directions, divider focus and deliberate unrelated focus changes. Independent review then caught the initial phone layout regressing before the deferred module loaded. Added a startup-only responsive fallback and an actual blocked-module browser regression; later resize transitions remain controller-owned. Reviewed source pin: 9c4ef9f2870896939e3592894fd9664975de7d52. Startup red failed before the fix (0.603s package); rebuilt-assets focused green passed all 10 selected tests with no skips/failures (18.141s): AgentPanesBrowser, AgentPaneBreakpointFocusBrowser, AgentPaneInitialLayoutBrowser, GalleryPageScriptSuite, AgentPreview* and DirectOpenPreviewSwapBrowser. Independent fresh Standards and Spec round5 reviews accepted the startup fix and preservation of earlier fixes; zero new actionable findings. Retained non-blocking pre-existing P3: the preview Details link has a source-inferred resize focus edge outside the pane/tab-strip/divider transition; not independently reproduced. Current screenshots still represent the verified geometry. Parent owns final combined verification; no physical iPhone/VoiceOver testing, merge or deployment.

**2026-11-02 refresh**

Rebased onto main 9b8f08df after the library navigation stack merged. Production changes retain their original patch identity. A fresh overlap review raised relay consent inside the hidden Preview pane; an actual browser check disproved the suspected trap. The real relay controller opens its nested modal with visible, hit-testable controls, Decline focus, and return focus to the composer, both while agent.js is delayed and after Preview becomes inert. Added TestAgentPaneRelayConsentBrowser to retain this compatibility coverage and to the required browser gate; all three test/subtest results passed with no skips. No consent UI or permission behavior changed. Parent owns final combined verification and fresh delta review.
