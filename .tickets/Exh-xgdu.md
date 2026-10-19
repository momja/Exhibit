---
id: Exh-xgdu
status: in_progress
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

