---
id: Exh-oeaf
status: closed
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


## Notes

**2026-10-24T05:22:47Z**

Implemented Edit permissions on active saved native API-key/Bearer/Basic cards. Owner-only, trusted-human PATCH changes only allow_writes, preserves exact encrypted credentials, other recipe fields, identity, grants, usage, cache and state, and serializes with authorized requests/revocation. Sorted capabilities enter every attached artifact's authority binding so stale hosts must reload. Save uses the existing server-rendered htmx refresh; Cancel sends nothing and failures stay inline. Provider OAuth scopes still require renewed provider consent.

Validation: focused connector/store tests and race checks passed; API ownership, cross-site/agent denial, original credential use and multi-artifact stale-authority tests passed, including focused API race. Real Chromium covers save/refresh, enable/disable, Cancel and revoked-record failure. All 395 gallery tests, changed-package vet and server build passed. The broad Go run completed with 23 passing packages and 7 optional/no-test skips; its only failures were two icon-subset checks in API. Switched the button to the already registered Phosphor sliders icon, then reran both failed checks plus the connection API/page/browser suite successfully. No remaining test failure; the entire broad suite was not repeated after this icon-only correction.

Updated API, relay and architecture docs and desktop/mobile screenshots. Deployed the demo and inspected the real Github editor without changing its saved credential, permissions, grants, approvals or artifact state. No scope deviation: this edits the local native HTTP-method permission, not provider-side token scopes.
