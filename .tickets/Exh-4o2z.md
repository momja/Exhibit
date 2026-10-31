---
id: Exh-4o2z
status: in_progress
deps: []
links: []
created: 2026-10-31T20:55:42Z
type: task
priority: 2
assignee: Max Omdal
---
# Ship Pi Bolt as the Exhibit agent runtime

Replace the Docker runtime Pi-on-Node installation with pinned Pi Bolt 1.0.8 (Pi 1.4.0), load unchanged Exhibit extensions at runtime, support Linux amd64/arm64 with verified release archives, update runtime docs, and validate agent/API compatibility plus the built image. Open a focused PR against current origin/main; no deployment.

## Acceptance Criteria

Pinned verified Bolt release in shipping image; pi and PI_BIN contract retained; existing agent, guardrail, metering, source, and resume tests pass; container runtime smoke passes; documented rollback and focused PR.

