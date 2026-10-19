---
id: Exh-5hmx
status: closed
deps: []
links: []
created: 2026-10-19T16:05:35Z
type: bug
priority: 1
assignee: Max Omdal
---
# Apply direct-open confirmation to the agent view

PR #163 missed the agent preview Open link. Route agent-view direct opens through the shared confirmation, preserving per-viewer/artifact remembered choices and supporting preview swaps after saves and artifact changes.

## Acceptance Criteria

Agent Open shows the shared confirmation; cancellation opens nothing; confirmation uses the current fresh-token /open route with noopener; remembered choices match the detail view; swaps and newly created artifacts work; focused tests and screenshots verify the behavior.


## Notes

**2026-10-19T16:10:49Z**

Applied the shared direct-open gate to the agent button, including server-rendered artifact/URL data, swap-safe delegation, per-viewer/artifact preferences shared with detail, and focus restoration. Validation: all 304 gallery JavaScript tests, API/render Go suites, asset build and server build passed. T3 browser verified cancel/focus, preview swap, one isolated new tab, remembered choice and mobile sizing. Desktop/mobile screenshots included.
