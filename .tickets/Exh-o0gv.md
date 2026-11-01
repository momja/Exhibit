---
id: Exh-o0gv
status: closed
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

**2026-11-01T18:06:15Z**

Completed and rebased onto main 3c45711. All five findings have regression coverage. Failed managed saves and abandoned flows retain durable remote cleanup ownership; ordinary browser GET caching and no-cache/no-store behavior work; schedules verify enabled accounts before fetch and save; cache expiry/count/byte limits are enforced with covering-index metadata pruning. Reconnect identity and grants remain intact, and flow locks are released safely after failed setup. Full go test -json -timeout 20m ./... passed (23 tested packages, 2,713 passing cases); go vet ./..., make assets, all 439 gallery tests and focused connector lifecycle/lock race tests passed. Metadata pruning measured 34 ms at a 256 MiB cache versus 4.8 s before optimization. Documentation updated; no UI layout changed, so current screenshots remain applicable. Live provider validation and indeterminate provider-response reconciliation remain in Exh-3kvx.
