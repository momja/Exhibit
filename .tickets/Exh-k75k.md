---
id: Exh-k75k
status: open
deps: [Exh-v6v4, Exh-hz3g]
links: []
created: 2026-07-12T03:30:17Z
type: task
priority: 3
assignee: Max Omdal
parent: Exh-i0ll
---
# Extract the agent into its own repository (exhibit-agent service)

Stand up the exhibit-agent repo: move internal/agent (manager, sessions, pi sidecar spawn, ext/exhibit.ts), internal/secrets + agent_keys storage (the agent service owns BYO-key sealing in its own datastore), the /agent chat UI, the agent API routes, and cmd/mockllm into a standalone Go service. Config: EXHIBIT_URL + EXHIBIT_TOKEN (service token for API calls), PI_BIN, its own listen port/secret. The browser talks only to the agent service; the agent service proxies all Exhibit reads/writes through the HTTP API with its token — no CORS on Exhibit, token never in page JS. Then remove the agent code, agent_keys migration, and agent handlers/UI from Exhibit core (transcripts table + endpoints and the snippet picker stay — they are core). Keep docs/agent.md in the new repo; Exhibit docs keep a short pointer. Plan: docs/agent.md §Extraction.

