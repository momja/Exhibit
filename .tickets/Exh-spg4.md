---
id: Exh-spg4
status: in_progress
deps: [Exh-mg8f]
links: []
created: 2026-10-22T06:50:51Z
type: feature
priority: 2
assignee: Max Omdal
parent: Exh-g2vv
tags: [experimental, connectors, security]
---
# [Experimental] Phase 2: generic native connectors (API key, Bearer, Basic)

EXPERIMENTAL; part of Exh-g2vv; builds on phase 1.

- **Data model:** connections (owner or viewer, provider/recipe, kind, encrypted secret using the agent_keys AES-GCM pattern, created_at, last_used_at, status) and per-artifact connection grants (which connection, plus where else the artifact may send data).
- **Recipes:** declarative, shipped in the binary, reviewed like code: origins, how to acquire, where the secret goes (header and scheme), quota and cache, sensitivity. Generic API key / Bearer / Basic are filled in from the user's choices in the connect UI. No provider-specific recipes.
- **Credential engine interface:** the relay asks the engine to authorize a request for a connection; native first, vendor in phase 4.
- **Attach-by-origin:** the artifact calls the API without credentials, and the relay injects them only for the recipe's origins and only into the recipe's designated header. No stand-ins yet (phase 5).
- **Acquisition only in Exhibit's UI:** a connect page (paste a key, a username and password, or a URL with embedded credentials, which is split into origin and Basic credentials), and a manage page (revoke, last used, which artifacts use each connection).
- **Grant prompt:** generated mechanically from the recipe: 'can read your X; can send data to: ...'. Financial recipes default to 'nowhere else'.
- **Ownership:** connections belong to the viewer by default; sharing yours with share recipients is an explicit opt-in.
- **Quotas:** optional per-connection limits the user sets (at most N requests per day; reuse responses for M minutes), enforced by the relay for every artifact using the connection. Some APIs disable tokens that exceed a daily budget, and repeated page opens would otherwise exhaust it. Every connection defaults to "data may go nowhere else".
- **Agent tools:** list_connections (names, providers, origins, capabilities; never secrets) and request_connection(provider, reason), which prompts in Exhibit's UI. Agents never write recipes.
- **Note:** fetch() rejects URLs with embedded credentials, so clients must send a Basic header; the connect UI's URL splitting covers access URLs of that shape.
- **Plumbing:** account deletion covers the new tables (TestEveryTableIsAccountedForInAccountDeletion); exports and bundles never contain secrets.
- **Widgets:** may not use the relay in v1; they show state the artifact keeps fresh.

## Acceptance Criteria

- An agent-built app using a Basic-auth connection (created by pasting a URL with embedded credentials) shows real data. The credential never appears in frame memory, state, the state inspector, exports, agent transcripts or share renders.
- A request to a non-recipe origin never carries the credential.
- A share recipient never uses the owner's connection unless it was explicitly shared.
- Exceeding the connection quota is refused before reaching the provider.

