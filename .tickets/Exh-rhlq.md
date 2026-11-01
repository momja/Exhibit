---
id: Exh-rhlq
status: in_progress
deps: [Exh-354p]
links: []
created: 2026-10-27T18:33:48Z
type: task
priority: 3
assignee: Max Omdal
parent: Exh-17a5
tags: [mcp, auth, oauth, self-hosted]
---
# Self-hosted MCP authorization path

Built-in minimal OAuth 2.1 authorization server in the core, active only when MCP_URL is set (epic Exh-17a5 Architecture).

- Endpoints: /.well-known/oauth-authorization-server (RFC 8414), /oauth/register (open dynamic client registration, public clients, stateless signed client_id), /oauth/authorize (consent page in the page group), /oauth/token (authorization code + PKCE S256, resource must equal MCP_URL).
- The person signs in with whatever login the instance has (local, OIDC, Clerk, or none); the consent page names the client and says it will get access to their whole library (search, create, read and edit artifacts and saved data). No artifact picker.
- The token is a library grant (Exh-l2xj) labelled with the client name; no expiry, no refresh token; it is listed and revoked on /profile like a pasted one.
- No vendor SDK; owning auth stays small (technical_stack.md section 10).

## Acceptance Criteria

With MCP_URL set, the go-sdk OAuth client completes registration, consent and token exchange and then edits an artifact through the MCP service; without MCP_URL every /oauth and authorization-server metadata route is 404.
