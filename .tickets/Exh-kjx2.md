---
id: Exh-kjx2
status: in_progress
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

