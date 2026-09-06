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

**2026-09-06T01:21:09Z**

DECISIONS (2026-09-05): read widening, and what a share row becomes

## Non-owner read is a parallel accessor, not a widened predicate

ownsArtifact and the owner-scoped EXISTS subquery stay exactly as they are.
Reaching a shared artifact goes through a separate, deny-by-default accessor
naming only the routes a recipient may use.

The rejected alternative was widening the existing predicate to "owner, or named
on a live share". One change and every query picks it up, including DELETE, the
body rewrite and share revocation — so a recipient would inherit the ability to
destroy the artifact, and the failure would be silent. Deny-by-default is the
shape agentSubResources and public mode's route allowlist both already use, for
this reason.

## A directed share is a GRANT, not a link

The prior question, which the earlier "one row or many recipients?" framing hid.

    link   the unguessable id is the secret; you send /s/abc123
    grant  the row says "user 7 may read artifact X"; they open /artifacts/X

Grant. The unguessable id exists because an anonymous link has nothing else to
check at the door; a named grant has identity there, so the secret buys nothing
and adds a way to lose access — a recipient who drops the bookmark is locked out
of something they were granted. It also collapses the URL space: the recipient's
link, their "shared with me" row and the owner's own link all point at
/artifacts/:id.

The anonymous link stays a row in the same table with no recipient, where the
secret id is still the whole authorization, and /s/:shareID stays its door.

So: one row per recipient, because a row IS a recipient.

## Schema

    ALTER TABLE shares ADD COLUMN recipient_id INTEGER REFERENCES users(id) ON DELETE CASCADE;
    ALTER TABLE shares ADD COLUMN state_mode   TEXT NOT NULL DEFAULT 'own';
    CREATE UNIQUE INDEX shares_artifact_recipient ON shares(artifact_id, recipient_id);

recipient_id IS NULL is the anonymous link; set, it is a grant. The unique index
makes "one grant per (artifact, person)" a schema invariant rather than a
convention — the argument artifact_network_origins already makes for its primary
key. Two grants with different state modes should be unrepresentable.

The cascade means a recipient deleting their account takes their grants with it,
and migration 014's users trigger already retires their state rows.

This supersedes the av-20xv note from earlier today: shares.public should be
DROPPED, not wired up. It becomes exactly `recipient_id IS NULL`, so keeping it
is two columns encoding one fact with no way to stop them disagreeing. Same
reasoning av-8ipt used on expires_at.

**2026-09-06T01:23:15Z**

CORRECTION (2026-09-05): state_mode belongs on the artifact, not the grant

Raised by the group-chat case: give several friends access, drop ONE URL in a
group chat, each of them opens it in their own account.

The grant model already delivers that and it is worth stating outright, since
"one row per recipient" reads as "one URL per recipient" and is not:
**the artifact URL is the share URL.** /artifacts/:id is the same address for
the owner and every grantee, because the link does not carry the grant. Only
the anonymous /s/:shareID model puts the capability in the URL. Someone in that
chat without a grant gets a 404; someone without an account hits the login gate
and then a 404 as themselves.

The alternative the case seemed to ask for — one row per artifact holding a
recipient list — costs more and expresses less. SQLite has no arrays, so it
becomes shares(artifact_id, ...) plus share_recipients(share_id, user_id): two
tables to say what one grant row says, and revoking a person becomes a list edit
rather than a DELETE.

## What it does change

state_mode was wrong on the grant. "Shared board or private copies" is one
question with one answer per artifact, and per-grant admits Alice on her own
list while Bob writes the owner's. Nobody wants that state and it makes "whose
board am I on" unexplainable on the page.

    ALTER TABLE artifacts ADD COLUMN share_state_mode TEXT NOT NULL DEFAULT 'own';
    ALTER TABLE shares   ADD COLUMN recipient_id INTEGER REFERENCES users(id) ON DELETE CASCADE;
    CREATE UNIQUE INDEX shares_artifact_recipient ON shares(artifact_id, recipient_id);

av-v991's 18:42 note put the flag on the share row so revoking a share would
revoke write access in one motion. Under grants that argument is spent: deleting
a grant revokes that person outright, so the mode sits where it reads correctly.

## And granting is a bulk action

"Gave friends access, dropped the link" is one gesture. The UI is an add-people
field taking several names and one submit, not one person at a time. That pushes
the still-open "how does the owner name a recipient" question toward a
multi-entry typed field.

**2026-09-06T01:26:52Z**

DECISION (2026-09-05): one anonymous link per artifact

The anonymous share is a single link, not a set. Grants are per person; the
public link is per artifact.

**The unique index proposed earlier does not enforce this.** SQLite treats every
NULL in a unique index as distinct from every other NULL, so
UNIQUE(artifact_id, recipient_id) accepts any number of recipient-less rows for
one artifact. It constrains grants and says nothing about the link. Two indexes:

    CREATE UNIQUE INDEX shares_artifact_recipient
      ON shares(artifact_id, recipient_id);
    CREATE UNIQUE INDEX shares_one_anonymous_link
      ON shares(artifact_id) WHERE recipient_id IS NULL;

The partial index makes it a schema fact rather than a handler convention. No
precedent in the tree (idx_tags_owner_name is the only unique index today), but
SQLite has had partial indexes since 3.8.

## Consequences for the UI

- **Minting is a toggle, not a button.** "Public link" is a switch: on creates
  the row and shows the URL, off deletes it and the URL dies. Nothing
  accumulates a list of links whose destinations nobody remembers.
- **Revoke has exactly one meaning**, which is the whole reason not to allow a
  set of them.
- **Rotation needs its own control.** A leaked link wants replacing, and
  toggling off then on gets there while leaving the user unsure it worked. A
  "replace link" action that deletes and re-mints in one step, stating that the
  old URL stops working, earns its button.

The id stays random and unguessable: for that row the URL is still the entire
authorization.

**2026-09-06T03:55:12Z**

RELEASE NOTE (2026-09-05): av-6axy changes the render token wire format

The named-claim encoding (av-6axy, merged to the epic branch) is a clean cutover
rather than a compatible extension, which the design sanctioned: "accept a short
TTL window and cut over cleanly (TTL is 10 minutes; in-flight tokens age out
fast)".

So on the deploy that ships it, **render tokens minted by the previous binary
stop verifying.** Exposure is narrow — a link a user clicks later goes through
/artifacts/:id/open, which mints on redirect — and what breaks is a page held
open across the deploy whose frames or htmx fragments reload inside the ten
minute window. Those frames 404 until the page is reloaded.

Worth one line in the release notes; not worth a compatibility parser.

Also correcting an acceptance criterion I wrote on av-6axy: "a token minted with
no p and no a is byte-identical to what this package mints today" is not
achievable, since changing the encoding is the ticket. The design note's
"byte-identical" sentence is about p defaulting to o. The tests pin the
meaningful version instead: an ordinary mint is exactly o=<owner>.e=<unix>.<tag>
and nothing more.

And "sorted key order" was built as a fixed canonical order (o, e, p, a, s)
rather than lexicographic, which would emit e= before o= and contradict every
documented wire example. Same property, one claim set to one byte string.
