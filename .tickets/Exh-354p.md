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
# OAuth resource-server support in the MCP service

The half of MCP authorization the resource owns, now in the exhibit-mcp service (Exh-uhvu) rather than the core.

- GET /.well-known/oauth-protected-resource (RFC 9728, both the path-inserted and bare forms) on the service, naming the resource (MCP_PUBLIC_URL) and the core's APP_ORIGIN as its authorization server (EXHIBIT_PUBLIC_URL, defaulting to EXHIBIT_URL).
- A 401 carries WWW-Authenticate: Bearer resource_metadata=...
- Tokens are verified by the core (GET /api/agent/credential); the service never decides validity itself. Every accepted token is a library grant; the core's authorization server only issues grants for MCP_URL as resource (Exh-rhlq).
- Document the path-routing case: when the service lives at APP_ORIGIN/mcp, the reverse proxy must route /.well-known/oauth-protected-resource/mcp to the service too.
