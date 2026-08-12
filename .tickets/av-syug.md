---
id: av-syug
status: in_progress
deps: []
links: [av-wmp6, av-30rj, av-c5aq, av-5imk, av-ep8k]
created: 2026-08-09T04:31:48Z
type: bug
priority: 1
assignee: Max Omdal
tags: [security, multi-user, gallery, middleware]
---
# Gallery pages are not owner-scoped: any logged-in user browses owner 1's library

The av-swzv epic made `owner_id` a real predicate on every API query. The **page** routes never got it.

`/`, `/new`, `/artifacts/:id`, `/artifacts/:id/edit` and `/agent` are registered at the top level of `setupRoutes`, outside the `ro.Group(authMiddleware, ownerMiddleware)` block — they always have been, because they are HTML and the page's own JS authenticated its API calls separately. `sessionGate` guards them, but it calls `ro.sessionUser(r)` and **discards the owner**: `if _, ok := ro.sessionUser(r); ok`. It propagates only a boolean (`withSessionAuthed`). Nothing puts the session's owner into the page request's context.

So `ownerIDFromCtx` hits its fallback and returns `defaultOwnerID` (1). A user whose `owner_id` is 2 logs in, loads `/`, and is served **owner 1's library**. Worse, `renderURLs` mints that page's frame tokens from the same value, so av-c5aq's `a.OwnerID == principal` check passes and owner 1's artifacts render — bodies and inlined state — inside user 2's gallery.

This is the exact class of cross-tenant read the epic exists to prevent, on the one surface the epic did not cover.

**Not exploitable today**, because every owner is 1. It becomes live the instant a second user exists — i.e. exactly when av-30rj's OIDC path or the hosted tier is used in anger. A multi-user deployment is unsafe until this lands.

Found while reviewing av-5imk, which deliberately did not fix it: that ticket was scoped to the page *credential*, and this is the page *owner*. The two are adjacent but separate, and av-5imk's `gallery.go:92` carries a comment acknowledging the gap.


## Notes

**2026-08-09T04:31:48Z**

DESIGN

The fix is small; the reason it was missed is the interesting part, and the tests should encode it.

**Make the session's owner reach the page request.** `sessionGate` already resolves the user — it is throwing the value away. Put it in the context the same way `authMiddleware` does for API routes (`context.WithValue(ctx, ownerIDKey, ownerID)`), so `ownerIDFromCtx` returns the real owner on page routes too. `ownerMiddleware`'s existing 'never overwrite an owner resolved upstream' rule means the two compose without ordering surprises.

Then confirm the downstream users are correct rather than assuming: `galleryIndex`, `galleryDetail`, `galleryEdit`, `galleryNew`, `agentPage`, the `/partials/*` fragment routes, `openArtifact`, and `renderURLs` (whose `ownerID` decides which principal a frame token names).

**Why the epic's tripwires did not catch this.** `TestEveryArtifactScopedMethodTakesAnOwner` guards the *Store* interface, and `TestUnscopedAccessorsAreCalledOnlyFromTheRenderSurface` guards the unscoped accessors. Both are about the data layer. Nothing asserts that a *handler* passes the requester's real owner rather than a default — and `ownerIDFromCtx`'s silent fallback to 1 is what makes the omission invisible: there is no error, no zero value, just the wrong library.

That fallback is worth reconsidering as part of this. A function that quietly answers 'owner 1' when nobody set an owner is a footgun on a multi-tenant instance; failing closed (or returning `(int64, bool)` and forcing the caller to decide) would have made this a compile error or a 401 instead of a silent cross-tenant read. Note `ListArtifacts` already fails closed on an unset `OwnerID` — the store layer got this right and the context helper did not.

## Acceptance criteria

1. A session-authenticated page render uses the session's owner; a user with `owner_id` 2 never sees owner 1's artifacts on any page route.
2. Frame tokens minted on a page name the session's owner, so a second user's gallery cannot render another owner's artifact or its state.
3. Single-user instances (no identity provider) are unchanged — owner stays 1 and the existing suite passes untouched.
4. Anonymous public visitors (av-wmp6) continue to resolve to `PUBLIC_OWNER_ID`, not to the session default.
5. A test that fails if a page handler is added that renders artifacts without the requester's owner — in the shape of the existing route-walk guards (`csrf_test.go`, `renderheaders_test.go`, `pagecredential_test.go`), which fail on undeclared routes rather than passing vacuously.
6. Decide explicitly whether `ownerIDFromCtx` should keep its silent default, and record the reasoning either way.
