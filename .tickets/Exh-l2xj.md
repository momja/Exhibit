---
id: Exh-l2xj
status: closed
deps: []
links: []
created: 2026-10-27T18:33:48Z
type: feature
priority: 2
assignee: Max Omdal
parent: Exh-17a5
tags: [agent, mcp, auth, store]
---
# Persisted, owner-issued library grants for external agents

Durable counterpart to agentscope's process-lifetime grants, so an agent outside the server can hold a credential for its owner's library (epic Exh-17a5, revised 2026-10-31).

- Table agent_grants (next free migration number, see technical_stack.md section 3): id, token_hash (SHA-256, never stored in clear), owner_id, label, client (the OAuth client name, empty for a pasted token), created_at, last_used_at. No artifact_id. Removed with the owner (account deletion audit); no expiry, matching shares.
- Mint returns the plaintext token exactly once. Session only (not AUTH_TOKEN unless the instance has no login, not an agent grant, not a recipient).
- authMiddleware resolves a persisted grant to a library-scoped agentscope.Grant (owner, no artifact). agentScopeAllows gains a library mode: GET /api/artifacts (list/search), POST /api/artifacts (create), GET/PATCH /api/artifacts/:id, widget read/write, saved-state read only (never PUT/DELETE: state is not versioned) and asset metadata reads on any artifact the owner owns (owner scoping in the store turns anything else into 404), GET /api/agent/credential. Refused: shares, grants, origins, versions/restore, DELETE of an artifact, tags/collections, account, admin, and every page route. Pi's one-artifact mode is unchanged.
- PATCH by an agent keeps refusing share_state_mode (existing rule).
- Agent writes record origin=agent; message is the grant label, session id is grant:<id>.
- Routes: GET/POST /api/agent-grants, DELETE /api/agent-grants/:id (owner-only; another owner's id is 404). Not reachable by an agent credential.
- UI: a "Connected agents" section on /profile (Go partial + htmx refresh on exhibit:agent-grants-changed): label, client, created, last used, revoke; copy-once token (connection configs arrive with the MCP service, Exh-uhvu). The artifact page's "Connect your own agent" menu item is removed.

## Acceptance Criteria

A minted token can list/search, create, read and edit any of its owner's artifacts and their widget, and read their saved state and assets; writing or erasing saved state is refused; another owner's artifact is 404; an artifact shared with the owner is 404; shares, grants, origins, versions, deletion, account and admin routes are refused; revoking makes the next request 401; tokens are stored hashed; tests cover each refused route family.
