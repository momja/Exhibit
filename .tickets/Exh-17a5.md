---
id: Exh-17a5
status: closed
deps: []
links: [Exh-yvhp, Exh-wi2y, Exh-6bgx]
created: 2026-10-27T18:33:48Z
type: epic
priority: 2
assignee: Max Omdal
tags: [agent, mcp, auth, security]
---
# MCP access for bring-your-own agents

Let a user point their own agent (Claude Code, Claude Desktop, Cursor, Codex, ...) at their Exhibit library through MCP, so a durable agent can find an artifact by what it is ("my water tracker", "the run logger") and change it, with the authority of the person who connected it.

## Why the existing pieces are not enough

- The API is already the single write path and the Pi extension tools (internal/agent/ext/exhibit.ts) are effectively an MCP tool surface. Revision tokens (expected_source_revision, targeted edits) already give an external writer safe concurrency.
- Gallery search already exists: GET /api/artifacts?q= runs the owner-scoped FTS5 query over title, visible source text and tags. A search tool is a thin call over it.
- But the scoped credential (internal/agentscope) is in-memory, minted only for a Pi subprocess, and revoked when that process exits. An external agent has no way to obtain one.
- The Clerk epic (Exh-wi2y, closed) does not supply MCP's OAuth: it verifies the __session cookie only and ignores Authorization headers.

## Trust model (revised 2026-10-31)

A connected agent acts for the person who connected it, across their whole library, the same way a signed-in laptop does. Exhibit's job is to require authentication to connect, to make every connection listable and revocable, and to keep the agent on the API's ordinary rules. Securing the agent itself (what other tools it has, what it is told to do, where its token is stored) is the user's job, as with any device they leave signed in. A one-artifact credential was dropped: it made the durable-agent case unusable and did not protect the library from the person who holds it.

## Invariants

- One credential, one owner's library. A grant resolves to its owner; it reaches that owner's own artifacts and nothing else: no artifacts shared with them, no other owner, no account, admin, share, or grant routes. A route outside the agent allowlist (agentScopeAllows, widened from one artifact to the owner's library) is refused.
- Owner-issued, listable, revocable. Only a signed-in person can mint a grant (or the operator on a no-login instance); every live grant is listed on the account page with its label, client and last use, and revoking it makes the next request a 401.
- Nothing new on the write path. MCP tools call the existing HTTP API; agent writes are versions with origin=agent and provenance (message, session) from the grant row, never from the request.
- Untrusted artifact text reaches the external agent only as tool results (av-5s7g). The server instructions say so; the user's agent decides what to do with it.
- MCP is optional. The MCP service is a satellite, like Litestream and Chromium: an operator who does not run it carries no MCP transport and exposes no MCP or OAuth endpoints, and their users are offered no MCP connection at all. There is one way in: the remote endpoint (no local connector; dropped 2026-10-31, see Notes).
- Self-hosters get the feature without Clerk. OAuth is a layer on top, not a prerequisite.

## Architecture

- **Core (the Exhibit server):** persisted library grants (store + routes + account-page panel), GET /api/agent/credential (what a token is), and, only when MCP_URL is configured, the built-in OAuth authorization server and the remote config shown in the panel.
- **exhibit-mcp (the MCP service):** speaks Streamable HTTP as the operator's optional MCP service (Compose profile `mcp`, routed at MCP_URL by the reverse proxy). It runs the tools from internal/mcpserver and reaches Exhibit only through its HTTP API with the caller's grant token. The service holds no database and no secrets of its own; it verifies each bearer by asking the core API, and serves the protected-resource metadata that names the core as authorization server.

## Tools

search_artifacts (new: query, optional tags; returns id, title, tags, updated_at), create_artifact, get_artifact, edit_artifact, write_artifact, get_state (read-only), get_widget, set_widget, edit_widget. Every per-artifact tool takes artifact_id. Not exposed: writing or erasing saved data (state is not versioned, so an agent change to it could not be undone; the route policy refuses it too, not only the tool list), deletion, restore, shares, tags/collections, asset upload, and Pi-only get_selection/ask_user.

## Stories (stacked in this order)

1. Exh-l2xj: Persisted library grants: mint/list/revoke from the account page, authMiddleware resolves them to the owner's library.
2. Exh-q6xo: MCP tools: artifact_id on every tool, search_artifacts, create_artifact.
3. Exh-uhvu: The MCP service: `exhibit-mcp`, Compose profile, MCP_URL in core; no MCP transport in the core binary.
4. Exh-354p: OAuth resource-server half in the MCP service: protected-resource metadata, WWW-Authenticate, token check through the core.
5. Exh-rhlq: Built-in authorization server in core (only with MCP_URL); consent approves library access.
6. Exh-vora: Hosted Clerk as authorization server: superseded by Exh-rhlq, pending the owner's decision.
7. Exh-xlju: Security and docs.

Stories 1-3 deliver library-wide BYO-agent editing with a pasted token (sent as an Authorization header); 4-5 add OAuth so clients sign in through the browser instead.

## Out of scope

- Artifacts shared with the user (incoming grants); a library grant reaches the owner's own artifacts only.
- Writing or erasing saved data, deleting artifacts, restoring versions, sharing, and network/capability decisions through MCP tools.
- Replacing the Pi harness; it keeps its process-lifetime, one-artifact grants.

## Notes

**2026-10-31** Revised after the stack was opened as PRs #234-#239 and before any merged: one credential now covers the owner's library (with search_artifacts), and the remote endpoint moved out of the core into an optional MCP service. The PRs are rebuilt in place per story.

**2026-10-31** Dropped the local stdio connector (`exhibit-mcp` on the user's machine with a pasted token). It existed only so users of instances without the MCP service could connect; a self-hoster who wants MCP is their own operator and can enable the service as easily, every mainstream client speaks remote MCP, and the connector needed the Go toolchain and left the token in plain text in the client config. Without MCP_URL the instance offers no MCP.
