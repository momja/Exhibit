---
id: Exh-o0gv
status: in_progress
deps: []
links: []
created: 2026-11-01T17:31:30Z
type: bug
priority: 1
assignee: Max Omdal
parent: Exh-g2vv
---
# Fix connector lifecycle, cache, and disabled-account review findings

Fix the five PR #197 review findings: durable cleanup for failed managed account completion; empty GET cache handling; request and response cache directives; disabled-account schedule authorization; bounded persistent cache retention.

## Acceptance Criteria

All five findings have regression coverage, appropriate checks pass, and operator documentation records cache and cleanup behavior.


## Notes

**2026-11-01T17:43:30Z**

Implemented the five findings with regression coverage: durable managed-account cleanup and restart retries; normal browser GET response reuse; request/response cache directives; disabled-account schedule guards before fetch and save; encrypted cache retention caps with metadata-only pruning. Focused checks pass. Final rebase and broad checks in progress. UI layout is unchanged, so existing connector screenshots remain current.
