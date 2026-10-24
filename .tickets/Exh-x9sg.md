---
id: Exh-x9sg
status: in_progress
deps: [Exh-poaf]
links: []
created: 2026-10-20T06:24:35Z
type: feature
priority: 1
assignee: Max Omdal
parent: Exh-xvtt
---
# Private admin-token artifact bundle import

Create an unadvertised account-targeted API requiring a dedicated configured admin token. Reuse PR #184 state/archive helpers and staging/quota patterns. Validate every entry and CRC before writes, remap artifact and asset IDs/references, create source/widget/state/assets atomically, preserve cleanup on failure and reset sensitive permissions. Test auth, malformed archives, ownership, round trips, rollback and quotas.

