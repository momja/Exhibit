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

Create an unadvertised account-targeted API requiring the existing operator AUTH_TOKEN as a bearer credential. Reuse PR #184 state/archive helpers and staging/quota patterns. Validate every entry and CRC before writes, remap artifact and asset IDs/references, create source/widget/state/assets atomically, preserve cleanup on failure and reset sensitive permissions. Test auth, malformed archives, ownership, round trips, rollback and quotas.


## Notes

**2026-10-20T06:41:58Z**

Implemented dedicated-token-only account import outside ordinary API auth, strict ZIP/CRC/metadata/state validation, fresh artifact/asset/generation/blob identities with cross-origin reference rebinding, atomic enabled-account Store.ImportArtifactBundle publication, quota preflight and durable staging cleanup. Added hand-built archive, auth, fidelity, compressed expansion, quota, rollback and cleanup recovery tests; exporter schema and limits coordinated without schema edits.

**2026-10-20T09:03:56Z**

Integrated the dedicated-token-only importer with versioned ZIP exports and explicit existing-account targets. Source/widget/generation/asset identities are fresh; asset blobs use destination-account content addresses so Stored Assets export/restore and quota deduplication remain compatible. Auth refusals, malformed archives, rollback/cleanup, quotas, exact owner state and cross-instance historical-asset restoration pass the focused suite.

**2026-10-20T20:33:10Z**

Per user preference, private import now reuses the existing AUTH_TOKEN bearer credential and the shared matchesServiceToken check. Removed the separate import token configuration and startup environment setting. Sessions alone, scoped agent credentials and query credentials remain refused before lookup/body inspection. The server and Compose retain their existing dev-token fallback when AUTH_TOKEN is unset or empty; this does not disable import. Updated operator instructions and cross-instance/authorization/body-limit tests.
