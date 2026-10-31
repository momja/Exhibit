---
id: Exh-w0l5
status: in_progress
deps: []
links: []
created: 2026-10-31T16:44:09Z
type: task
priority: 2
assignee: Max Omdal
---
# Compile-test Exhibit custom Pi tools under Pi Bolt

Inventory Exhibit Pi extensions; install pinned current Pi and Pi Bolt; build an isolated identical-tools RPC harness, compile and execute every tool, and measure fresh-process startup, short request latency, streamed CPU and RSS. Record reproducible results, verbatim diagnostics, blockers, and recommendation in docs. No product/auth changes or binary distribution.

## Acceptance Criteria

Clear all-tools compile/run verdict; at least 10 startup observations per runtime with median and spread; deterministic request and long-stream measurements; per-tool runtime dependencies and compatibility; reproducible harness and cited Markdown report.

