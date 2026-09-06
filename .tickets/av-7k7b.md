---
id: av-7k7b
status: open
deps: []
links: [av-buyx, av-q0ub, av-wrbu, av-wmp6, av-v991, av-0k5q, av-ec0t, av-20xv, av-6xjd]
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

**2026-09-05T16:53:14Z**

CORRECTION (2026-09-05): the "no shim on shared renders" scope decision is dead

The scope decisions at the top say a share gets no shim, no inlined owner state,
and is read-only end to end. Two things have overtaken that.

**Shipped behaviour already differs.** render.ServeShare passes
Claims{OwnerID: a.OwnerID} into serveArtifactDoc, so a share inlines the owner's
state today and installs the preamble like any other render. Only the write
vanishes, because persistState returns early on window.parent === window. The
acceptance criterion "no shim script and no owner state in the served document"
describes something the code has not done for a while.

**The product moved the other way.** av-0k5q chose directed shares over saved
copies: an artifact keeps its single owner, a recipient runs it and writes its
state, and nothing is ever copied or re-editable by them. av-v991 takes the
anonymous share to a writable shared board, one board per link, on the owner's
rows. A share carrying live state is now the direction rather than a leak to
close.

**What survives from this epic:**

- A share UI that mints, lists and revokes. Still the missing product (av-6xjd).
- The enumeration problem. A share forgotten eight months ago and still carrying
  live state is the failure mode worth designing against, and it got worse with
  this decision, not better.
- The visitor's half. Someone handed a link opted into nothing and has to be
  told plainly what the page does with what they type.

Rewrite the scope decisions and acceptance criteria against av-v991's state
modes before anyone starts building.

**2026-09-06T00:43:33Z**

V1 SCOPE (2026-09-05), after the av-0k5q decision

Sharing v1 is directed shares between accounts on this instance. Stated as a
list, because three earlier notes each describe a different shape:

- Every artifact has exactly one owner. Sharing never transfers it and never
  copies the file.
- A share is directed at an account on THIS instance. Exhibit is not
  decentralized, so a recipient is a users row, and identity at both ends is
  the premise rather than a limitation to work around.
- The recipient may run the artifact and write its state through use. They may
  not copy it into their gallery, edit its source, change its metadata, or
  re-share it.
- An anonymous link stays read-only: it inlines the owner's state and persists
  nothing, which is what ships today.
- No expiry. av-8ipt removed expires_at and nothing here adds it back. The
  earlier recommendation to default state-carrying shares to a deadline is
  deferred, not adopted.

## Out of scope for v1: the anonymous writable link

av-v991 records the park chess board (one shared board per anonymous link, on
the owner's rows) and that design stands. Building it does not happen in v1.

What that buys, and it is worth naming because two open tickets were waiting on
it: v1 has no path on which an unauthenticated stranger writes to the operator's
database. Every writer holds an account. So av-wrbu's ceilings and the missing
rate limiter stay ordinary hygiene rather than blockers, and the "not a place
for critical data" warning copy is not needed on any surface v1 ships.

## What v1 still has to answer

Two accounts on one board is in scope (a directed share in 'shared' state mode),
so the liveness problem is not deferred with the anonymous link. See av-v991.

**2026-09-06T01:02:20Z**

V1 SCOPE ADDENDUM (2026-09-05)

Two additions to the scope list above.

**Only the owner modifies CSP, allowlists and capability approvals.** Already
enforced server-side by owner-scoped queries (av-ep8k), so this is a client
rule: a recipient's session renders none of the host-frame prompts that write
per-artifact authority. It explains instead of asking, the way av-kmwj already
handles a violation approval could not fix. Detail and the failure mode on
av-6xjd.

**The recipient's surface is the app-origin detail page, not /s/:id.** They hold
an account, so they get the ordinary framed page. That removes three pieces from
v1 that earlier notes on av-v991 designed: no SSE on the render origin, no
connect-src system source, and no share-scoped write credential. The host frame
already carries both directions.

The prerequisite this creates: a non-owner cannot reach the detail page at all
today, because GetArtifact is owner-scoped and 404s. That is the "letting a
non-owner reach a shared artifact" line in architecture.md §8's evolution table,
and it moves onto v1's critical path.
