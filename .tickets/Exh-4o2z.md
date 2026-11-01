---
id: Exh-4o2z
status: closed
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


## Notes

**2026-10-31T21:20:05Z**

Implemented pinned Pi Bolt 1.0.8 / Pi 1.4.0 in Docker with verified ARM64 and amd64 baseline hashes, full release assets/licenses, pi/pi-bolt aliases and no runtime Node/npm. ARM64 image builds and passes all 12 custom tools (normal and guard-enabled), prepared-source edit, server startup and authenticated API. Agent suite passed. Full API execution had 1212 passes, 3 optional S3 skips and one async transcript-readiness assertion; fixed the test to verify artifact head then await matching transcript version, passed 25 repetitions and follow-up Agent/Pi/Guardrail/Conversation/Resume API suite (49s). No application/auth/session code changes. amd64 archive checksum verified; native amd64 execution not available on this ARM64 host.
