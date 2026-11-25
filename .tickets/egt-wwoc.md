---
id: egt-wwoc
status: closed
deps: []
links: []
created: 2026-11-25T07:59:06Z
type: chore
priority: 2
assignee: Max Omdal
tags: [agent, runtime, docker]
---
# Update pinned Pi Bolt to 1.3.1 (Pi 1.6.0)

Bump the pinned agent runtime from Pi Bolt 1.0.8 (Pi 1.4.0) to Pi Bolt 1.3.1 (Pi 1.6.0) in the Dockerfile with verified release archive hashes, and update the docs that name the pin.

## Acceptance Criteria

Dockerfile pins PI_BOLT_VERSION=1.3.1 with the published linux-x64-baseline and linux-arm64 archive SHA-256 hashes; README, docs/agent.md, docs/deployment.md and docs/technical_stack.md name Pi Bolt 1.3.1 / Pi 1.6.0; the real-Pi API and agent suites pass with Pi 1.6.0 on PATH; no application code changes.


## Notes

**2026-11-25T09:03:31Z**

Bumped the pin to Pi Bolt 1.3.1 (Pi 1.6.0) with the release SHA256SUMS hashes for linux-x64-baseline and linux-arm64, and updated README, docs/agent.md (plus a note on Pi 1.5/1.6 extension-surface changes), docs/deployment.md, docs/inference.md and docs/technical_stack.md. Verified: docker pi-bolt stage hash check OK and reports '1.6.0 (Pi-Bolt 1.3.1, linux-arm64, JIT off)'; full image builds and ships pi/pi-bolt symlinks to /opt/pi-bolt/pi with no node; internal/agent passes against host pi 1.6.0 and against the image's extracted Pi Bolt binary; internal/api full suite passes against host pi 1.6.0 (680s) and against the image's Pi Bolt binary (608s), with only the three optional S3 skips. One first-run flake in TestGalleryFilterBrowser/phone under full-suite load passed 3/3 alone and on both clean full runs; no application code changed.
