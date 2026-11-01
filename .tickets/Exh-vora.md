---
id: Exh-vora
status: closed
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


## Notes

**2026-10-30T08:00:23Z**

Likely superseded by Exh-rhlq (2026-10-30). The built-in authorization server authenticates the person with whatever session the instance has, Clerk's included, so hosted MCP clients already complete OAuth without a pasted token, and every token is an artifact grant by construction. Using Clerk as the AS instead would need a Clerk-JWT verifier, a mapping from (Clerk user, OAuth client) to a grant, and an artifact-picking step Clerk's own consent screen cannot host. Left open for the owner to close or to keep if Clerk-issued tokens are wanted for another reason.

**2026-10-31T05:40:08Z**

Revised epic (2026-10-31): grants are now library-wide, so a Clerk OAuth token for a user would map onto a grant without an artifact step. The built-in authorization server (Exh-rhlq) still covers hosted instances through the Clerk session, so this remains superseded unless Clerk-issued tokens are wanted for another reason.

**2026-10-31T16:10:20Z**

Closed 2026-10-31: not doing. The built-in authorization server (Exh-rhlq) covers hosted instances through the Clerk session on the consent page, and every MCP token is an Exhibit library grant. Clerk-issued access tokens would add a second token type for the MCP service to verify and a second place where connections live, with no gain for users.
