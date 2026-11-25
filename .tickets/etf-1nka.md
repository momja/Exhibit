---
id: etf-1nka
status: closed
deps: []
links: []
created: 2026-11-22T01:00:03Z
type: bug
priority: 1
assignee: Codex guardrail subagent
external-ref: gh-307
parent: Exh-zue3
tags: [inference, security]
---
# Screen every platform-paid artifact inference request

Artifact inference must apply the configured Exhibit usage-policy guardrail to all provider-bound content at each paid dispatch. Current screening misses content in completion schemas, classifier criteria, and structured-output repair requests.

## Acceptance Criteria

Screen the full provider-bound input for complete and decide, including the repair input, whenever platform guardrails apply. Refused or failed screens prevent that dispatch. Preserve viewer and instance cap checks and meter every dispatched screen. Add focused tests with benign mock fixtures for both kinds and the repair path. Verify without live providers.


## Notes

**2026-11-22T01:07:17Z**

Verified full-context screening before initial completion, classification, and structured repair. Both spend caps are checked before each screen and again after an allowed screen is metered. The inference race suite passed with the pinned Pi Bolt 1.0.8 worker and a local mock provider. Focused API and ledger tests and the mock provider tests passed.
