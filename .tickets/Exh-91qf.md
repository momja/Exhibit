---
id: Exh-91qf
status: in_progress
deps: []
links: []
created: 2026-11-08T19:14:59Z
type: chore
priority: 2
assignee: Max Omdal
---
# Remove redundant Starred tab description

The Starred gallery shows the heading "Starred" followed by the line "Your go-to artifacts, most recently opened first." The heading already names the destination, and the ordering promise is not needed to use the tab. Remove the extra copy so the tab opens straight into the artifact grid.

## Acceptance Criteria

The Starred gallery renders only its heading, with no description line beneath it; other gallery destinations keep their descriptions; gallery tests and Exh-77m9 screenshots reflect the change.

