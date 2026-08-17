---
id: av-ke2m
status: closed
deps: []
links: [av-q30x, av-30rj]
created: 2026-08-06T15:25:11Z
type: bug
priority: 2
assignee: Max Omdal
tags: [security, auth, middleware]
---
# CSRF protection is implicit: SameSite=Lax is load-bearing but undocumented and untested

av-30rj introduced cookie authentication. `authMiddleware` accepts a session for **any** method, so the cookie authenticates mutating routes (POST/PUT/PATCH/DELETE) exactly as it does reads. The bearer-token model this replaces had no CSRF exposure at all — an attacker's page cannot set an Authorization header — so this is new surface.

The protection today is `SameSite=Lax` on the session cookie (internal/api/auth.go). That is genuinely sufficient, for two reasons that both have to hold:

1. Lax withholds the cookie on cross-site *unsafe* methods, so a cross-origin POST arrives with no credential and 401s. (Explicitly-set Lax also avoids Chrome's 'Lax+POST' two-minute exception, which applies only to cookies with no SameSite attribute.)
2. Lax *does* send the cookie on cross-site top-level GET navigations — which is safe only because no GET route mutates. Every r.Get in the API group is a read: list, detail, state, widget, transcripts, key, collections, tags.

So there is no live vulnerability. The problem is that both conditions are load-bearing and neither is written down or pinned by a test. Two ordinary future changes silently remove the protection:

- Someone sets `SameSite=None` to make an embed or a cross-origin client work.
- Someone adds a mutating GET (a `GET /api/artifacts/:id/refetch` convenience route is exactly the shape that would seem harmless).

av-q30x's open questions already asked for CSRF to be 'decided explicitly'. It shipped decided-implicitly.


## Notes

**2026-08-06T15:25:11Z**

DESIGN

Do not add CSRF tokens. SameSite=Lax already provides the protection and a token layer would be redundant machinery on a single-origin, cookie-plus-bearer API. The work is to make the existing decision explicit and enforced.

Three parts:

1. **Write it down.** docs/security.md gains a short CSRF section stating that SameSite=Lax is the mechanism, that it is sufficient only while no GET route mutates, and that changing SameSite is a security change rather than a compatibility tweak. Put it beside the session material av-30rj added.

2. **Pin the cookie attribute.** A test asserting the session cookie is SameSite=Lax already exists (internal/api/auth_test.go around line 201) — it just reads as an attribute check rather than as the CSRF control. Give it a name and a comment that say what it is defending, so someone loosening it sees why they cannot.

3. **Pin the no-mutating-GET invariant.** This is the part that does not exist. Walk the API route tree with chi.Walk and assert every registered GET handler is a read. The mechanism can be as simple as an explicit allowlist of GET routes that the test requires to match exactly — the same shape av-nr0p used for Referrer-Policy coverage, which fails loudly when a route is added rather than passing vacuously. A new mutating GET should fail the suite with a message naming CSRF.

Note for whoever picks this up: av-wmp6 (public mode) will make some GET routes unauthenticated. That does not change this — an unauthenticated GET carries no credential to abuse — but the test's allowlist will need to survive that change, so prefer 'every GET is a read' over 'every GET is authenticated'.
