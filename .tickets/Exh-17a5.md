---
id: Exh-17a5
status: open
deps: []
links: [Exh-yvhp, Exh-wi2y]
created: 2026-10-27T18:33:48Z
type: epic
priority: 2
assignee: Max Omdal
tags: [agent, mcp, auth, security]
---
# MCP access for bring-your-own agents

Let a user point their own agent (Claude Code, Claude Desktop, Cursor, Codex, ...) at one artifact and edit it through MCP, with the same authority a built-in Pi session gets and no more.

## Why the existing pieces are not enough

- The API is already the single write path and the Pi extension tools (internal/agent/ext/exhibit.ts) are effectively an MCP tool surface. Revision tokens (expected_source_revision, targeted edits) already give an external writer safe concurrency.
- But the scoped credential (internal/agentscope) is in-memory, minted only for a Pi subprocess, and revoked when that process exits. An external agent has no way to obtain one.
- The Clerk epic (Exh-wi2y, closed) does not supply MCP's OAuth: it verifies the __session cookie only, requires azp == APP_ORIGIN, ignores Authorization headers, and is hosted-only. Clerk can act as an OAuth authorization server (DCR or client ID metadata documents), but its tokens authorize a user, i.e. the whole library. The Go SDK (v2.7.0) has no verifier for Clerk-issued OAuth tokens.

## Invariants

- One credential, one artifact. Every MCP credential resolves to (owner, artifact) and passes through agentScopeAllows; no tool takes an artifact id. A user-wide token (cookie, AUTH_TOKEN, raw Clerk OAuth token) never reaches the artifact routes from /mcp.
- Owner-issued, listable, revocable. Only a session can mint a grant; the owner can see and revoke every live one. Enumeration is not optional (same reasoning as sharing).
- Nothing new on the write path. MCP tools call the existing HTTP API; agent writes are versions with origin=agent and provenance from the grant.
- Untrusted artifact text reaches the external agent only as tool results (av-5s7g applies unchanged).
- Self-hosters get the feature without Clerk. OAuth is a layer on top, not a prerequisite.

## Stories (stacked in this order)

1. Exh-l2xj: Persisted artifact grants: mint/list/revoke, authMiddleware resolves them.
2. Exh-q6xo: Stdio MCP server using a pasted grant token.
3. Exh-uhvu: Remote /mcp endpoint (Streamable HTTP) with bearer grant token.
4. Exh-354p: OAuth resource-server half: protected-resource metadata, WWW-Authenticate, pluggable token verifier.
5. Exh-vora: Hosted: Clerk as authorization server, exchanged for an artifact grant through an Exhibit consent page.
6. Exh-rhlq: Self-hosted authorization path (decision + implementation).
7. Exh-xlju: Security and docs: security.md section 5, agent.md, api.md.

Stories 1-3 deliver usable BYO-agent editing for everyone. 4-6 remove the token paste for clients that require OAuth.

## Out of scope

- MCP tools that span artifacts or list the library.
- Replacing the Pi harness; it keeps its process-lifetime grants.

