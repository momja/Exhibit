---
id: Exh-oeaf
status: in_progress
deps: []
links: []
created: 2026-10-24T05:04:22Z
type: feature
priority: 2
assignee: Max Omdal
parent: Exh-g2vv
tags: [experimental, connectors, permissions]
---
# [Experimental] Edit permissions on saved native connections

Saved native API-key, Bearer and Basic connections currently expose only Revoke. Add a trusted owner-only edit path for allow_writes so people can enable or disable POST/PUT/PATCH/DELETE without re-entering credentials, recreating the connection or removing its artifact grants. Keep relay consent separate. Provider OAuth scopes are still managed through reconnection.

## Acceptance Criteria

Saved active native connections have Edit permissions and a prefilled API changes checkbox, save and cancel controls. Saving updates only allow_writes, preserves the encrypted credential, identity, origin/header/base, grants, cache/quota and state, and exposes safe updated capabilities. Other owners, shared recipients, artifact agents and cross-site requests cannot change it; managed/revoked/missing records fail safely. Permission changes invalidate stale host authority for every artifact using the connection. Document where to edit connector versus relay permissions; verify live UI and backend tests, then push.

