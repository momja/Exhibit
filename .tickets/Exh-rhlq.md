---
id: Exh-rhlq
status: open
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

Decide and implement what self-hosters get for clients that require OAuth.

Options to evaluate:
- A minimal built-in authorization server (authorization code + PKCE, client ID metadata documents, issuing the persisted grant as the token) using the existing session for user authentication and the same artifact consent page.
- Using OIDC_ISSUER as the authorization server where the IdP supports it (most do not do per-resource scoping).
- Stay on pasted tokens (Exh-q6xo, Exh-uhvu) and document it.

Constraint from technical_stack.md section 10: no vendor SDK, and owning auth must stay small.

