---
id: Exh-vora
status: open
deps: [Exh-354p]
links: []
created: 2026-10-27T18:33:48Z
type: feature
priority: 3
assignee: Max Omdal
parent: Exh-17a5
tags: [mcp, auth, oauth, clerk, hosted]
---
# Hosted: Clerk as MCP authorization server with artifact consent

On hosted Exhibit, let MCP clients complete OAuth without a pasted token.

- Enable Clerk's OAuth applications for the instance; prefer client ID metadata documents, DCR only if needed (DCR forces the consent screen on).
- Verify Clerk OAuth access tokens: local JWT verification against the instance JWKS (iss, exp, aud/resource), or the /oauth_applications/access_tokens/verify REST endpoint. The pinned clerk-sdk-go v2.7.0 has no helper; implement in internal/auth next to clerk.go.
- Artifact consent: during authorization the user picks one artifact on an Exhibit page; the result is a persisted artifact grant (Exh-l2xj) bound to that OAuth client, and the token verifier maps the Clerk token + client to that grant. A Clerk token with no grant is refused.
- Grants created this way appear in the artifact's External agents list with the client name, and revoke there.

