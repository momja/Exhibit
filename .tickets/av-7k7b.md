---
id: av-7k7b
status: open
deps: []
links: [av-buyx, av-q0ub, av-wrbu, av-wmp6, av-v991]
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


## Notes

**2026-08-05T04:50:42Z**

Correction to the "Known consequence, accepted for v1" scope decision (2026-08-04).

That decision says native `localStorage` throws a SecurityError on a shim-less shared render, so "storage-using tools may not work when shared" — and plans UI copy saying so. That is true of the **framed, opaque-origin** case, not of how shares are actually opened.

`/s/:shareId` is viewed **top-level**: the app-origin route redirects straight to the render origin (internal/api/api.go serveShare), and a recipient clicking a share link lands there as a top-level document. A top-level render-origin document has a real, stable origin, so native `localStorage` works normally and persists across reloads. The opaque origin — and therefore the SecurityError — only applies inside the sandboxed iframe.

So dropping the shim on `/s/:id` is not a degradation for the primary flow. It is strictly better than what happens today: the visitor gets a working, persistent, device-local tool instead of a shim whose writes silently vanish on reload (the same footgun av-blzu documents for path 2). And it closes the owner-state leak by construction rather than by remembering to filter.

Consequences for this epic:
- The planned share-UI warning copy would be misleading for the common case. If a warning is kept at all it should be conditional on the framed case, or reworded to "your changes stay on this device" rather than "may not work".
- The acceptance criterion "no shim script and no owner state in the served document" is unchanged and still right.
- This is the same reasoning already documented for sessionStorage in spec §5.2: install a replacement only where the native surface is broken or wrong; leave it alone where it works. The symmetry is a good sign the decision is right for the right reason.

Related: av-q0ub adds the `(artifact, principal)` state key that makes "whose state gets inlined" expressible rather than implicit, and would let a future `state_mode` on the share row offer an explicit "share a read-only snapshot of my data" option. Not needed for this epic; noted so the default-closed choice here stays compatible with it.
