---
id: Exh-rhlq
status: closed
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


## Notes

**2026-10-30T08:00:18Z**

Decision: a minimal built-in authorization server (option 1). DCR (RFC 7591, open, public clients only) + authorization code with PKCE S256 + RFC 9207 iss + RFC 8707 resource checked against APP_ORIGIN/mcp. The consent page (/oauth/authorize, a page behind sessionGate) lists the signed-in owner's artifacts; the owner picks one. The access token IS a persisted agent grant (Exh-l2xj) labelled with the client's name, listed and revoked on the artifact page; no expiry, no refresh token. Nothing is stored for clients: a client_id is its registration signed with a key derived from the server secret, so open registration cannot grow a table. Codes are in memory, 5 minutes, single-use. Because it authenticates the person with the existing session (local, OIDC or Clerk), it serves hosted instances too. OIDC_ISSUER as AS was rejected (IdPs do not scope per resource); pasted tokens remain for every client. CIMD not implemented (DCR covers Claude Code, Cursor, VS Code). Verified end to end with the go-sdk's own OAuth client (discovery, DCR, consent, exchange, /mcp). Screenshot: docs/screenshots/Exh-rhlq/01-consent.png
