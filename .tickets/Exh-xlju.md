---
id: Exh-xlju
status: in_progress
deps: []
links: []
created: 2026-10-27T18:33:48Z
type: chore
priority: 2
assignee: Max Omdal
parent: Exh-17a5
tags: [docs, security, mcp]
---
# Document MCP access: security model, agent and API docs

- security.md section 5: the external-agent trust argument as revised (a connected agent is the owner's signed-in device: library-wide, owner-issued, listable, revocable, ordinary API rules; securing the agent is the user's job; untrusted artifact text reaches it as tool results only; state erasure is not versioned).
- agent.md: the external path next to Pi, finding artifacts with search_artifacts, and what each tool does.
- api.md: agent-grant routes, GET /api/agent/credential, the OAuth endpoints.
- architecture.md: section 3.7 (MCP service as a satellite), section 3.6 satellites, and the evolution-seams table.
- deployment.md: running the MCP service (Compose profile, MCP_URL, proxy routing).
- README: setup snippets for common clients (remote only).
