---
id: Exh-uhvu
status: open
deps: [Exh-l2xj]
links: []
created: 2026-10-27T18:33:48Z
type: feature
priority: 2
assignee: Max Omdal
parent: Exh-17a5
tags: [agent, mcp, api]
---
# Remote /mcp endpoint with bearer grant token

Same tool surface as the stdio server, served in-process at APP_ORIGIN/mcp over Streamable HTTP, so clients that only speak remote MCP work without a local binary.

- Authenticates only with Authorization: Bearer <persisted grant>. Session cookies and AUTH_TOKEN are refused on /mcp (no ambient browser authority, no CSRF surface).
- Tool handlers call the same internal handlers/store paths as the HTTP API (or loop back through the router) so the single write path holds; decide which in design.
- Respect MAX_REQUEST_BODY_BYTES and the server's no-write-timeout listener; check Origin per the MCP transport spec.
- Share the tool definitions with the stdio server.

## Acceptance Criteria

A remote MCP client with a pasted grant token can edit its artifact; a cookie-only or AUTH_TOKEN request to /mcp is 401.

