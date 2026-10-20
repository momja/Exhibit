---
id: Exh-x9sg
status: closed
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


## Notes

**2026-10-20T06:41:58Z**

Implemented dedicated-token-only account import outside ordinary API auth, strict ZIP/CRC/metadata/state validation, fresh artifact/asset/generation/blob identities with cross-origin reference rebinding, atomic enabled-account Store.ImportArtifactBundle publication, quota preflight and durable staging cleanup. Added hand-built archive, auth, fidelity, compressed expansion, quota, rollback and cleanup recovery tests; exporter schema and limits coordinated without schema edits.
