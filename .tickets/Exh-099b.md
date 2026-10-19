---
id: Exh-099b
status: closed
deps: []
links: []
created: 2026-10-19T05:12:10Z
type: epic
priority: 2
assignee: Max Omdal
tags: [agent]
---
# Make the agent ready sooner and eliminate redundant source reads

Coordinate five PRs: upgrade Pi; eagerly prepare the session and source after deliberate typing; correct prompts that cause redundant source reads; retain source snapshots and send targeted edits instead of whole documents on every edit; enforce optimistic concurrency for agent and human source/widget writes. Preserve tool-result roles for untrusted source and per-artifact capability scope. No model/action benchmarking, thinking summaries, or speculative inference while typing. Delegate independent work, review every PR with review-agent, then prepare an integrated stacked review.

## Acceptance Criteria

Five individually reviewed PRs form a coherent stack. Source preloading avoids a model read round trip without placing artifact data in instruction roles; edits reuse known snapshots, conflicts save nothing, and existing session/guardrail/resume behavior remains covered.


## Notes

**2026-10-19T06:25:37Z**

Implemented all five children and assembled PR stack #169 → #170 → #171 → #172 → #174. Every PR passed an individual read-only review-agent review after fixes. Combined beefacf…4aadf77 review: No findings; 25 focused tests passed. Final rebased go test -timeout=20m ./... and asset build passed; real-Pi and browser lifecycle checks passed. docs/agent-readiness-review.md provides the stack guide, security/OCC contracts, and validation. Full application Docker image build remains unverified; Node22/Pi1.3.0 runtime smoke passed. PRs remain open and nothing merged or pushed to main.
