---
id: av-wmp6
status: closed
deps: [av-4ac9, av-ep8k]
links: [av-7k7b, av-q0ub, av-5imk, av-rgp1, av-v991, av-wrbu]
created: 2026-07-09T06:04:24Z
type: feature
priority: 1
assignee: Max Omdal
parent: av-ec0t
tags: [backend, auth, public-mode, middleware]
---
# Backend: Conditional auth middleware for public mode

Modify the existing chi auth middleware to become public-mode aware. When PUBLIC_MODE_ENABLED is true, skip auth checks for safe read-only GET routes: GET /api/artifacts, GET /api/artifacts/:id, GET /api/settings/public, and the root gallery route. All mutating routes (POST/PUT/PATCH/DELETE) must remain auth-gated regardless of public mode. The unauthenticated gallery renderer needs to know it is in public mode so it can suppress edit controls.

## Acceptance Criteria

1. Unauthenticated GET requests to /api/artifacts succeed when public mode is enabled. 2. Unauthenticated GET requests to /api/artifacts/:id succeed when public mode is enabled. 3. Unauthenticated POST/PUT/PATCH/DELETE to any API route returns 401 even in public mode. 4. When public mode is disabled, all routes require auth exactly as before. 5. The auth middleware passes a public-mode flag to request context so handlers can branch.


## Notes

**2026-08-05T04:51:24Z**

Sequencing dependency added (2026-08-04): this now depends on av-ep8k (owner-scope the store).

Public mode makes unauthenticated `GET /api/artifacts` return the library. There is no owner in that request by definition — and `ListArtifacts` currently filters on no owner either (`ListOptions` has no owner field), so today the two happen to agree. That agreement is accidental and ends the moment `owner_id` is anything but 1: an unauthenticated public read would return *every* owner's artifacts.

Two ways to resolve it, and this ticket should state which:
- Land av-ep8k first, and have public mode read as an explicitly designated instance owner (`PUBLIC_OWNER_ID`, or the single owner of a single-user instance). Public mode then means "this owner's library is public", which is a coherent thing to say on a multi-tenant deployment too.
- Or declare public mode mutually exclusive with multi-user and enforce it at startup.

The first is preferable — it keeps public mode meaningful on a hosted instance (a user publishing their own library), rather than making it a single-user-only feature that has to be removed later.

Note this is a sequencing issue, not a live bug: with `owner_id` fixed at 1 there is exactly one library and public mode returns the right thing.

**2026-08-06T05:21:09Z**

DESIGN DECISION (2026-08-06) — resolved, do not re-litigate

Take option one from the note above: public mode reads as an explicitly designated instance owner. av-4ac9 now ships `PUBLIC_OWNER_ID` (integer, default 1) for exactly this, so this ticket inherits a named owner rather than inventing one. Concretely: when public mode is on and a request carries no credential, the owner middleware resolves the owner to `PUBLIC_OWNER_ID` instead of refusing. Public mode then means 'this owner's library is public', which stays coherent on a multi-tenant deployment.

The alternative — declaring public mode mutually exclusive with multi-user and enforcing it at startup — is rejected: it makes public mode a single-user-only feature that has to be torn out the first time someone wants to publish their own library on a shared instance.

---

UNRESOLVED, AND BIGGER THAN THE AUTH CHANGE: does a public gallery publish the owner's STATE?

This ticket predates av-c5aq and av-q0ub and does not mention state at all. It is now the main design question, and it is not answerable by the middleware alone.

The mechanics, after the av-swzv epic:
- Gallery cards embed `/w/:id?t=…` and detail pages embed `/a/:id?t=…`. Those tokens are minted server-side from the request's owner (av-c5aq).
- In public mode that owner is `PUBLIC_OWNER_ID`, so an unauthenticated visitor receives a valid render token for that owner's artifact — intended, that is what public means.
- But the render surface inlines that principal's state into the document (av-q0ub). So a public visitor opening a run tracker sees the owner's runs; a public card's widget tile renders them on the gallery grid without a click.

That is very likely NOT the intent. Note the contrast with shares: `ServeShare` deliberately passes `a.OwnerID` because 'a share publishes the artifact as its owner sees it' (architecture.md §7) — a considered per-artifact decision the user makes one artifact at a time. Public mode flips an entire library to that posture in one env var, which is a categorically larger blast radius for the same mechanism.

Three options, none free:
(a) Public renders inline no state — the artifact boots empty for anonymous visitors. Closest to 'a public tool', and it is already what av-7k7b wants for shared renders. Widgets on public cards would then render their empty state, which may look broken for state-driven tiles.
(b) Public renders inline the owner's state, i.e. today's behaviour, made explicit and documented as 'publishing your library publishes what is in it'.
(c) Per-artifact opt-in, so an artifact is public-with-state only if its owner said so — most correct, most work, and needs a column.

This overlaps av-7k7b directly (read-only publish with the storage shim disabled), so the two should be decided together rather than arriving at different answers for /s/:id and public mode. Linking them.

---

SECOND-ORDER: writes silently vanish for anonymous visitors

Mutating routes stay auth-gated in public mode (AC#3), which is right. But the storage shim's write-through path is `setItem` → postMessage → host frame → `PUT /api/artifacts/:id/state`, and the host's fetch ends in `.catch(function(){})`. An anonymous visitor's write will 401 and be swallowed: the value is in the in-memory cache so the tool looks like it worked, and it is gone on reload. Decide whether public mode should tell the visitor their changes are not being saved, or whether the shim should not persist at all for anonymous renders (which is option (a) above, from the write side). Either way it should be a decision, not an accident.

**2026-08-06T15:59:04Z**

RESOLVED on the feature branch (av-wmp6 implementation).

State exposure — option (a): an anonymous public render inlines NO state, and its shim persists none. This is a DEFAULT CHOSEN IN CODE, not a product decision that was made; a state-driven widget tile will render its empty state on a public gallery. If per-artifact opt-in (option c) is wanted, that needs a column and a new ticket.

Mechanism: rendertoken gains an optional trailing `a` claim (`<owner>.<expiry>.a.<tag>`), inside the MAC because it subtracts authority. `Verify` now returns `Claims{OwnerID, Anonymous}`. `renderURLs` mints the anonymous flavour whenever the request is marked a public visitor, so any page that later serves public visitors inherits statelessness rather than having to remember it.

Second-order (writes vanishing silently): fixed in the render preamble, not the host frame. `ANONYMOUS` short-circuits `persistState`, so the frame never posts a write the API would 401 into a swallowed `.catch`. One fact about one document, both halves in one template.

Routes opened: GET /api/artifacts and GET /api/artifacts/:id only, resolved to PUBLIC_OWNER_ID by ownerMiddleware. /state, /widget, /transcripts, collections, tags, shares and agent routes all stay authenticated, as does every mutating method.

Still open for the frontend tickets (av-eu3v/av-epnt/av-n8v5): the gallery PAGE routes sit outside the auth group and embed AUTH_TOKEN in their bootstrap script. Nothing marks a page request as a public visitor today (correctly — without an identity provider the operator and a visitor are indistinguishable there), so no page is anonymous-readable yet. Whoever builds the public page must both set the public-visitor mark AND stop emitting the token.
