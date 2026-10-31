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
# Persisted, owner-issued artifact grants for external agents

Durable counterpart to agentscope's process-lifetime grants, so an agent outside the server can hold an (owner, artifact) credential.

- New table (next free migration number, see technical_stack.md section 3): hashed token (never stored in clear), owner_id, artifact_id, label, created_at, last_used_at, expires_at (nullable). Cascades with the artifact and the owner; add to the account-deletion table audit.
- Mint returns the plaintext token exactly once. Owner-only, session-only (not AUTH_TOKEN, not an agent grant, not a recipient).
- authMiddleware gains a branch after the existing agent grant: a persisted grant resolves to the same agentscope.Scope and goes through agentScopeAllows unchanged. Never bindable to a different artifact; POST /api/artifacts (the unbound create) is not allowed for these.
- Agent writes record origin=agent; message/session provenance comes from the grant label, not the request.
- Routes: POST/GET/DELETE /api/artifacts/:id/agent-grants (owner-only, 404 for others). Not in agentSubResources, so a grant cannot mint or revoke grants.
- UI: an "External agents" section on the artifact page (Go partial + htmx refresh on an exhibit:agent-grants-changed event) showing label, created, last used, revoke. Copy-once token display with the MCP config snippet.
- Decide expiry default (proposal: none, matching shares; visibility instead of a timer).

## Acceptance Criteria

A minted token can GET/PATCH its artifact and its state/widget/assets per agentScopeAllows and nothing else; revoking it makes the next request 401; another owner's artifact id is 404; tokens are stored hashed; tests cover each refused route.


## Notes

**2026-10-30T04:21:44Z**

Expiry decision: no expires_at column. A grant lives until revoked, matching shares (av-8ipt); the External agents list on the artifact page is what keeps a forgotten one visible. Minting is allowed for a session, or for the page credential of an instance with no login (the static token or none), never an agent or recipient. A disabled owner's grants stop resolving without being deleted. Screenshot: docs/screenshots/Exh-l2xj/01-external-agents.png
