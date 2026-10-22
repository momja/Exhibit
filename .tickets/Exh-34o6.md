---
id: Exh-34o6
status: closed
deps: [Exh-spg4]
links: []
created: 2026-10-22T06:50:51Z
type: feature
priority: 3
assignee: Max Omdal
parent: Exh-g2vv
tags: [experimental, connectors, billing]
---
# [Experimental] Phase 3: native OAuth 2 providers and connection entitlements

EXPERIMENTAL; part of Exh-g2vv.

- **OAuth 2 per provider**, via golang.org/x/oauth2 (already a dependency): authorization code with PKCE, refresh, revocation. Only providers users actually ask for (candidates: GitHub, Notion, Google, Microsoft, Spotify). Each provider is a fixed cost: register the app and maintain scopes. Google's restricted scopes (Gmail, Drive) need a yearly CASA assessment, so choose scopes accordingly.
- **Entitlements:** connection limits per kind, through the existing per-owner entitlement resolver (architecture.md §3.8c: Router.resolveAllowance, admin-only writer, fail closed). Native secrets: generous or unlimited. Native OAuth: generous. Vendor-backed: small (around 3 on a $10 plan), with an add-on pack or higher tier. Refusals name the limit, like storage quotas.

## Acceptance Criteria

- Connect, refresh and revoke work for the first provider end to end.
- Limits are enforced at connect time and visible on /profile.


## Notes

**2026-10-22T16:01:53Z**

Implemented browser/session-bound GitHub OAuth with PKCE, refresh/rotation and revocation, plus native/OAuth/vendor/schedule entitlement overrides and profile visibility. End-to-end TLS fixture tests pass. Operator registration and live provider consent remain external validation in Exh-3kvx.
