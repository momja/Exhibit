---
id: Exh-kjx2
status: closed
deps: []
links: []
created: 2026-10-19T05:35:00Z
type: bug
priority: 2
assignee: Max Omdal
---
# Align agent composer controls and clarify button states

The agent composer textarea is taller than its Send and Stop buttons. Send looks enabled for blank input, and both icon-only buttons lack accessible names. Match the initial control heights, disable Send for whitespace-only input including after programmatic clearing, and label both buttons while preserving steering during streaming.

## Acceptance Criteria

Initial textarea and action buttons share a height on desktop and touch; multiline input still grows; Send disables for blank input and updates after typing and send; Send and Stop have accessible names; streaming steering remains available.


## Notes

**2026-10-19T05:37:18Z**

Implemented shared 40px composer controls with centered icons, accessible Send/Stop names and tooltips, and Send disabled for blank input. The composer update path handles typing, clearing after send, and the /new brief handoff; steering remains available during streaming. Validation: make assets; node --experimental-vm-modules --test *.test.mjs (212 passed); targeted internal/api page tests passed. T3 browser checks at 1280px and 390px verified 40px initial alignment, multiline growth and reset, whitespace disabling, the 16px touch font token, and sending/reset while streaming with a stubbed prompt response. Screenshots in docs/screenshots/Exh-kjx2 show streaming controls via setStreaming(true), without starting a model session.
