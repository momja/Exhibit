---
id: Exh-spg4
status: open
deps: [Exh-mg8f]
links: []
created: 2026-10-22T06:50:51Z
type: feature
priority: 2
assignee: Max Omdal
parent: Exh-g2vv
tags: [experimental, connectors, security]
---
# [Experimental] Phase 2: native connectors (API key, Bearer, Basic, SimpleFIN)

EXPERIMENTAL; part of Exh-g2vv; builds on phase 1.

- **Data model:** connections (owner or viewer, provider/recipe, kind, encrypted secret using the agent_keys AES-GCM pattern, created_at, last_used_at, status) and per-artifact connection grants (which connection, plus where else the artifact may send data).
- **Recipes:** declarative, shipped in the binary, reviewed like code: origins, how to acquire, where the secret goes (header and scheme), quota and cache, sensitivity. Generic API key / Bearer / Basic are filled in from the user's choices in the connect UI; SimpleFIN is built in.
- **Credential engine interface:** the relay asks the engine to authorize a request for a connection; native first, vendor in phase 4.
- **Attach-by-origin:** the artifact calls the API without credentials, and the relay injects them only for the recipe's origins and only into the recipe's designated header. No stand-ins yet (phase 5).
- **Acquisition only in Exhibit's UI:** a connect page (paste a key; paste a SimpleFIN setup token, then base64-decode it and POST the claim server-side), and a manage page (revoke, last used, which artifacts use each connection).
- **Grant prompt:** generated mechanically from the recipe: 'can read your X; can send data to: ...'. Financial recipes default to 'nowhere else'.
- **Ownership:** connections belong to the viewer by default; sharing yours with share recipients is an explicit opt-in.
- **Quotas:** per-connection quotas plus relay caching (SimpleFIN: at most 24 requests a day, or tokens get disabled; widgets and repeated page opens would otherwise exhaust it).
- **Agent tools:** list_connections (names, providers, origins, capabilities; never secrets) and request_connection(provider, reason), which prompts in Exhibit's UI. Agents never write recipes.
- **SimpleFIN notes (verified 2026-10-22):** /accounts and /claim send Access-Control-Allow-Origin: null and allow Authorization; fetch() rejects URLs with embedded credentials, so send a Basic header; 90-day windows; results carry an errlist to show users.
- **Plumbing:** account deletion covers the new tables (TestEveryTableIsAccountedForInAccountDeletion); exports and bundles never contain secrets.
- **Widgets:** may not use the relay in v1; they show state the artifact keeps fresh.

## Acceptance Criteria

- A SimpleFIN budget app built by the agent shows real accounts. The access URL never appears in frame memory, state, the state inspector, exports, agent transcripts or share renders.
- A request to a non-recipe origin never carries the credential.
- A share recipient never uses the owner's connection unless it was explicitly shared.
- Exceeding the connection quota is refused before reaching the provider.

