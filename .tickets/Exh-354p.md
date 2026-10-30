---
id: Exh-354p
status: closed
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


## Notes

**2026-10-30T04:38:58Z**

The verifier seam is agentTokenVerifiers (oauth_resource.go): bearer -> agentToken{grant, bindingID} or nil, asked by both authMiddleware and /mcp. Today it holds the persisted-grant verifier only. Exh-rhlq's built-in authorization server issues persisted grants as access tokens, so it needs no second verifier; a verifier for tokens someone else issues must check the audience is APP_ORIGIN/mcp. PRM is served at /.well-known/oauth-protected-resource/mcp and at the bare well-known path; authorization_servers is empty until an AS exists.
