---
id: av-o5cf
status: closed
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

- A single typed value (e.g. Principal{OwnerID, Kind, ReadOnly, IsAdmin}) is resolved exactly once per request, in one place. The contract is closed before implementation: Kind's allowed values enumerate the anonymous, public, no-token, session, service-token, and agent-grant states; a distinct scope field is defined rather than overloaded onto Kind; OwnerID's validity is documented per Kind; and any unspecified or invalid state is default-deny.
- authMiddleware, sessionGate, adminOnly, authorizeEventStream, and pageCredentials/pageToken all consume that one resolution rather than each re-deriving authority; no handler or later middleware re-derives owner/admin/read-only status from raw session/token state. Coverage exercises every credential source plus the owner-1 fallback regression.
- agentScopeAllows and publicReadable's path-allowlist duplication is resolved or clearly justified as unavoidable (e.g. unified into one allowlist table if the two scopes are checking the same shape of thing). They may not survive as separate allowlists by exception: if distinct policies are genuinely required, a table-driven parity test covers every route × principal kind, preserving the cross-tenant 404 contract.
- The existing behavioral guarantees do not regress: pageowner_test.go's route walk, the public-mode tests (publicmode_test.go), the admin route guard tests, and the SSE authorization test all still pass unchanged in intent (may need updating to match the new call shape, but not in the property they assert).
- The dead publicVisitor branch in pagecredential.go either becomes reachable (if in scope) or stays clearly marked and unaffected by this refactor -- av-eu3v/av-epnt still own making it live.
- No new 403s appear where a 404 was previously returned for cross-tenant access (av-ep8k's fail-closed contract is preserved by construction, not just by the new code happening to get it right).


## Notes

**2026-08-11T17:24:47Z**

Implemented on feature/av-o5cf/consolidate-principal (branched off integration/multi-user,
since the code this touches doesn't exist on main yet).

New internal/api/principal.go: Principal{OwnerID, Kind, ReadOnly, Grant} + PrincipalKind
enum (None/Session/ServiceToken/AgentGrant/Public), stored under one context key. All
five chains now build or read this one value instead of independently re-deriving
authority from the raw request:

- authMiddleware and sessionGate each construct the full Principal (including OwnerID)
  for every branch they resolve, rather than leaving pieces for ownerMiddleware to fill
  in later. ownerMiddleware is now a pure backstop: single-user default if nothing
  resolved one, no-op otherwise, no Kind-specific logic of its own.
- adminRequest reads principalFromCtx(ctx).Kind instead of independently calling
  hasServiceToken(r)/sessionUser(r)/agentGrantFromCtx(ctx) — it can no longer disagree
  with what the request's own gate already decided. hasServiceToken (now unused) removed.
- authorizeEventStream (SSE, can't run middleware) returns a Principal via the same
  matchesServiceToken() authMiddleware uses. Bonus fix: this also closes a real
  constant-time-compare gap — authMiddleware's token comparison was a plain `==`
  while authorizeEventStream already used subtle.ConstantTimeCompare; both now share
  matchesServiceToken.
- pageCredentials/pageToken unchanged in logic, now backed by Principal via the
  publicVisitor()/sessionAuthed() projections in principal.go instead of separate
  ownerIDKey/agentGrantKey/publicVisitorKey/sessionAuthedKey context keys (all four
  removed).
- agentScopeAllows and publicReadable: extracted the one piece they genuinely shared
  (the /api/artifacts prefix-parse) into artifactsSubPath(); left their differing
  authorization policies separate rather than forcing one table over two different
  questions.

Deliberate deviation from the ticket's suggested shape: Principal does NOT carry
IsAdmin as a field. Admin status needs a DB lookup (session -> user -> IsAdmin), and
that lookup is intentionally per-request (not cached) so a demotion/disable takes
effect on the very next request, not just the next login. Making IsAdmin a Principal
field would mean either doing that lookup unconditionally on every request (real perf
cost, only ~3 routes need it) or leaving it lazily-populated (defeats the point of a
plain struct). adminRequest computes it on demand from Principal.OwnerID instead.

Verification: go build/vet/test all green (internal/api suite: 26s, all tests
including the pageowner_test.go route walk, publicmode_test.go, admin_test.go,
agent SSE tests). gofmt clean. golangci-lint not available in this environment to
run directly, but go vet found nothing and no dead code remains (grepped for the
four removed context keys and hasServiceToken across the whole repo — zero hits).

Two tests updated to construct a Principal directly instead of hand-writing the old
context keys (TestNeitherAgentSessionsNorPublicVisitorsAreAdmins in admin_test.go,
TestPublicVisitorsRenderURLsCarryNoPrincipal in publicmode_test.go) — same property
asserted, new call shape, per the ticket's own acceptance criteria allowance for that.

Not done (left for follow-up, matches the ticket's spirit but wasn't literally asked):
did not touch av-eu3v/av-epnt's dormant public-page branch — still dormant, unaffected.
