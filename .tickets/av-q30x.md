---
id: av-q30x
status: open
deps: []
links: [av-q3wo, Exh-mety, av-30rj]
created: 2026-07-06T22:06:13Z
type: epic
priority: 2
assignee: Max Omdal
---
# Add user authentication layer for future multi-user environments

Motivation: individuals should be able to secure their instance on public networks. This epic is **not** multi-user management — no registration, no invites, `owner_id` stays 1. It replaces the static bearer token (for browser use) with a real login, using the auth middleware seam that tech_stack §10 already reserved for exactly this.

## Scope decisions

- **v1 deliverable: single-user login page + signed-cookie sessions.** Username/password (single credential, hash stored server-side), session cookie checked by the same chi middleware that checks the bearer token today. The bearer token remains valid for programmatic clients (seed scripts, future extension, e2e fixtures).
- **Prefer established libraries over hand-rolling.** Evaluate before building: `alexedwards/scs` or `gorilla/sessions` for session management + `golang.org/x/crypto/bcrypt` (or argon2id) for the credential — these cover the hard parts without adding an identity server. Full OIDC (e.g. delegating to Authelia/authentik/an IdP) is deliberately out of scope for v1 — too much config surface for a single-user instance — but the middleware seam must not preclude adding it later.
- **Also document the BYO path.** For self-hosters who already run reverse-proxy auth (Authelia, Tailscale, basic auth at the proxy), docs should state that fronting the app origin with proxy auth is a valid alternative — consistent with the "TLS/proxy is the operator's" stance. In-app login is the default easy path, not the only path.
- **Render origin stays out of it.** `/s/:shareId` must remain no-auth (the share row is the authorization) and the render surface unauthenticated for shares; only the app origin's UI and API routes go behind the session.

## Open questions

- Where does the credential get set — env var at deploy, first-run setup page, or CLI flag? (First-run setup page is friendliest; env var is simplest.)
- Session lifetime / remember-me policy.
- CSRF: introducing cookie auth means mutating routes need CSRF protection that the bearer-token model didn't (same-site strict cookies may be sufficient — decide explicitly).


## Notes

**2026-08-05T04:51:24Z**

Scope boundary note (2026-08-04), from a multi-tenancy design pass. Nothing here contradicts this epic — it is correctly scoped as single-user login — but two of its decisions should not be inherited past that scope.

1. **"Render origin stays out of it" is valid only while `owner_id` is 1.** The reasoning given (the share row is the authorization, so `/s/:shareId` needs no auth) stays true forever. But `/a/:id` and `/w/:id` are also unauthenticated today, and with one owner that is a defensible trade against an unguessable UUID. With more than one owner it is a cross-tenant read of body *and* inlined state. Tracked as av-c5aq under the av-swzv epic. Flagged here so a future reader does not carry "the render origin is out of scope for auth" forward as a settled principle.

2. **Local password auth vs. delegating to a provider.** This epic picks username/password + scs/bcrypt and defers OIDC as too much config surface for a single-user instance. For a credential set once at deploy — no reset flow, no SMTP — that is defensible. The tension is that av-30rj (pluggable identity provider, for a hosted multi-user tier) needs a session layer with exactly the same shape: our own cookie, our own `sessions` table, the IdP touched only at `/auth/callback`.

   If this epic has not started yet, there is an argument for building that session layer once and making the local credential *one provider implementation* rather than the foundation — one session layer instead of two, and OIDC becomes additive. If it has already shipped, av-30rj slots a provider in behind it and nothing user-visible changes for single-user instances. Either order works; the one to avoid is building two independent session mechanisms.

Neither point needs action in this epic. They are recorded so the sequencing is a decision rather than a discovery.
