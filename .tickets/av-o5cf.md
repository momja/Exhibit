---
id: av-o5cf
status: open
deps: []
links: []
created: 2026-08-11T06:20:48Z
type: chore
priority: 1
assignee: Max Omdal
tags: [security, auth, architecture]
---
# Consolidate request-principal resolution into one typed value

Credential/authority resolution for a request is currently five independent, hand-rolled chains that must agree by discipline rather than by construction:

- authMiddleware (internal/api/middleware.go) — session -> service token -> agent grant -> public -> no-token pass-through
- sessionGate (internal/api/middleware.go) — its own login/path/session chain for page routes
- adminOnly/adminRequest (internal/api/admin.go) — a differently-ordered chain deciding admin authority
- authorizeEventStream (internal/api/agent.go) — a fourth chain that re-derives ownerMiddleware's default by hand for the one route (SSE) that can't run through normal middleware
- pageCredentials/pageToken (internal/api/pagecredential.go) — a fifth derivation, with one branch (publicVisitor on the page group) that is currently dead code because nothing sets it there yet

Plus two parallel deny-by-default path allowlists, agentScopeAllows and publicReadable (both in middleware.go), with near-identical CutPrefix("/api/artifacts") structure that has to be updated in lockstep whenever a new sub-route is added.

The invariant this is all supposed to guarantee -- every request resolves to exactly one (owner, scope, admin, read-only) -- exists only in comments and tests today, not in the type system. It has already broken once in exactly this way: av-syug shipped because page routes weren't wired to the session chain and silently fell back to owner 1 for every logged-in user; the pageowner_test.go route-walk test was built specifically to catch recurrences. av-rgp1 (SSE token in the URL) came out of the fourth chain being separate from the others. Today PUBLIC_OWNER_ID is honored on anonymous API reads but not on anonymous page reads, purely because ownerMiddleware's publicVisitor branch can't fire on the page route group -- that's the dead branch in pagecredential.go.

Upcoming tickets (av-g2dx account settings, av-7k7b shared artifacts, and any future API-key auth) each add a new principal shape that would otherwise need to be threaded through all five chains by hand.

## Acceptance Criteria

- A single typed value (e.g. Principal{OwnerID, Kind, ReadOnly, IsAdmin}) is resolved exactly once per request, in one place.
- authMiddleware, sessionGate, adminOnly, authorizeEventStream, and pageCredentials/pageToken all consume that one resolution rather than each re-deriving authority; no handler or later middleware re-derives owner/admin/read-only status from raw session/token state.
- agentScopeAllows and publicReadable's path-allowlist duplication is resolved or clearly justified as unavoidable (e.g. unified into one allowlist table if the two scopes are checking the same shape of thing).
- The existing behavioral guarantees do not regress: pageowner_test.go's route walk, the public-mode tests (publicmode_test.go), the admin route guard tests, and the SSE authorization test all still pass unchanged in intent (may need updating to match the new call shape, but not in the property they assert).
- The dead publicVisitor branch in pagecredential.go either becomes reachable (if in scope) or stays clearly marked and unaffected by this refactor -- av-eu3v/av-epnt still own making it live.
- No new 403s appear where a 404 was previously returned for cross-tenant access (av-ep8k's fail-closed contract is preserved by construction, not just by the new code happening to get it right).

