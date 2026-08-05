---
id: av-wmp6
status: open
deps: [av-4ac9, av-ep8k]
links: []
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
