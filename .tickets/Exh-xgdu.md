---
id: Exh-xgdu
status: closed
deps: []
links: []
created: 2026-10-19T05:52:48Z
type: bug
priority: 2
assignee: Max Omdal
---
# Agent History controls ignore system color preference

The Agent History button remains white when the operating system uses dark mode. History and continuation styles in web/gallery/agent.css still use literal light colors instead of the shared theme tokens.

## Acceptance Criteria

History button and History pane follow system light and dark colors, including live preference changes, while keeping the existing layout and hover behavior.


## Notes

**2026-10-19T05:55:40Z**

Replaced fixed light colors throughout History controls, conversation list and continuation card with shared palette and elevation tokens. Verified in the collaborative browser using existing server-side transcript fixtures: light/dark button styles match the key control, live scheme switches preserve performance.timeOrigin, list hover and continuation text/surfaces follow each palette. Saved screenshots under docs/screenshots/Exh-xgdu/. Validation: make build; 22 agent.history.test.mjs tests pass; focused internal/api History/Transcript/AgentPage/continuation tests pass; git diff --check.
