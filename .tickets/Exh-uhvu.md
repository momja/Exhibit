---
id: Exh-uhvu
status: closed
deps: [Exh-q6xo]
links: []
created: 2026-10-27T18:33:48Z
type: feature
priority: 2
assignee: Max Omdal
parent: Exh-17a5
tags: [agent, mcp, api]
---
# The MCP service: exhibit-mcp

The remote MCP endpoint as an optional satellite rather than part of the core binary (epic Exh-17a5 Architecture). Operators who do not want MCP run nothing and expose nothing.

- `exhibit-mcp` (cmd/exhibit-mcp, its only mode) serves the internal/mcpserver tools over Streamable HTTP. Config: EXHIBIT_URL (how the service reaches the core API, may be internal), MCP_PUBLIC_URL (its own public URL, the OAuth resource), listen address. It holds no database and no secrets.
- Authenticates only with Authorization: Bearer <grant>; every request's token is verified by GET /api/agent/credential on the core (no caching, so revocation holds on the next request) and forwarded as the API credential for tool calls. The MCP session is bound to the grant id: a different grant cannot continue it. Cookies are never read. Check Origin per the MCP transport spec; bound request bodies.
- Core: MCP_URL names the service's public URL. Unset means the instance offers no MCP: the settings section offers no token issuing and is hidden unless live grants exist (then listed for revocation only, with a note that MCP is not enabled). Set means the section issues tokens and shows the remote configs (Claude Code command, JSON). The core binary no longer contains an MCP transport (remove /mcp, the loopback API, and the go-sdk server dependency from internal/api).
- Packaging: the Docker image builds exhibit-mcp too (or a second slim image); docker-compose example gains an `mcp` profile; deployment.md shows routing /mcp (path on the app host) or a separate hostname to it.

## Acceptance Criteria

With the service running and MCP_URL set, a remote MCP client with a pasted grant searches and edits an artifact; without MCP_URL the core answers 404 at /mcp and the settings page offers no connection; a revoked grant fails its next request; a cookie-only request is 401.
