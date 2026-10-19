---
id: Exh-xco9
parent: Exh-099b
status: in_progress
deps: []
links: []
created: 2026-10-19T05:09:36Z
type: chore
priority: 2
assignee: Max Omdal
tags: [agent]
---
# Upgrade Pi to the latest stable release

Upgrade the pinned Pi sidecar from 0.99.1 to the latest stable release (verified npm latest: 1.3.0). Check release changes and maintain compatibility with Exhibit RPC, extensions, guardrail input handling, settlement, usage metering, and saved/resumed conversations. This is a prerequisite to the agent startup/source-preload work discussed in this thread; do not add model benchmarking or thinking summaries.

## Acceptance Criteria

Docker pins the verified latest stable Pi release; required Node runtime is compatible; real-Pi API pipeline, guardrail, conversation and resume tests pass; any compatibility changes and relevant documentation are included.

