---
id: Exh-t08t
status: in_progress
deps: []
links: []
created: 2026-10-30T03:01:10Z
type: chore
priority: 2
assignee: Max Omdal
---
# Remove unreachable Go code with deadcode

Run golang.org/x/tools/cmd/deadcode across all Go entry points, including tests. Review reported functions and remove obsolete code while retaining required interfaces and supported behavior.

## Acceptance Criteria

Reviewed unused functions removed; deadcode findings documented; relevant build and tests pass.

