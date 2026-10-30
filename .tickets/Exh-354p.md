---
id: Exh-354p
status: in_progress
deps: [Exh-uhvu]
links: []
created: 2026-10-27T18:33:48Z
type: feature
priority: 3
assignee: Max Omdal
parent: Exh-17a5
tags: [mcp, auth, oauth]
---
# OAuth resource-server support on /mcp

The half of MCP authorization Exhibit must own regardless of who issues tokens.

- GET /.well-known/oauth-protected-resource (RFC 9728) naming the resource (APP_ORIGIN/mcp) and its authorization server(s).
- 401 from /mcp carries WWW-Authenticate: Bearer resource_metadata=...
- A token-verifier interface: given a bearer token, return an agentscope.Scope or refuse. Implementations: persisted grant (Exh-l2xj), and the OAuth-issued token from Exh-vora and Exh-rhlq. Audience must be this resource; reject tokens issued for anything else.
- An OAuth access token for a user is never accepted as artifact authority by itself; it must resolve to an artifact grant (see Exh-vora).

