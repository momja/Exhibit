---
id: av-7k7b
status: open
deps: []
links: []
created: 2026-07-06T22:04:52Z
type: epic
priority: 2
assignee: Max Omdal
---
# Add sharing support for publishing an artifact to a read-only page with localstorage shim disabled

The share backend already exists (shares table + `GET /s/:shareId`, exhibit-7k3) but there is no UI to mint a share and the shared render currently goes through the same render surface as the owner's — which would inline the owner's state into a public page. This epic ships the user-facing sharing flow with that leak closed.

## Scope decisions

- **Access model (v1): public unguessable URL only.** `GET /s/:shareId` with a random ID, no auth, lives until the share row is revoked. Expiring links (`expires_at` UI) and the one-file `.html` export button are *not* in this epic — separate tickets if wanted.
- **No shim at all on shared renders.** The render surface omits the storage shim entirely for the `/s/:id` path: no inlined owner state (privacy), no postMessage write-through (there is no authenticated host frame to bridge writes anyway). The per-artifact CSP still applies unchanged.
- **Known consequence, accepted for v1:** in the opaque-origin sandbox, native `localStorage` access throws a SecurityError in Chrome, so an artifact that touches storage unguarded may break on its shared page while working in the gallery. Document this on the share UI ("storage-using tools may not work when shared") rather than engineering around it.

## Acceptance Criteria

- A share button on the artifact detail page mints a share row and surfaces the `/s/:shareId` URL; shares can be listed and revoked.
- The shared page renders with no shim script and no owner state in the served document (verifiable by inspecting the response body).
- Shared render is read-only end to end: no state writes occur, and the page works with no credentials in a fresh browser context.
- Revoked share IDs stop rendering.
- av-f05n's share.spec.ts switches from API-minted shares to the UI button once it exists (noted in that ticket).


## Complexity

XL
