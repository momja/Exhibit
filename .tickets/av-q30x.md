---
id: av-q30x
status: in_progress
deps: []
links: [av-q3wo, Exh-mety, av-30rj, av-ke2m, av-5imk, av-g2dx]
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

**2026-08-06T15:25:39Z**

RECONCILIATION WITH av-30rj (2026-08-06) — av-30rj has now shipped

Most of this epic's v1 deliverable already exists. av-30rj built the session layer, so the following are done and should not be rebuilt here:

- Session cookie + `sessions` table, opaque id looked up per request, revocable server-side.
- One chi middleware accepting **either** a session cookie or the static bearer token, so programmatic clients keep working exactly as this epic required.
- 'Render origin stays out of it' — and better than scoped: av-c5aq closed `/a/:id` and `/w/:id` with signed per-artifact tokens, while `/s/:shareId` stays no-auth because the share row is the authorization. The scope-boundary note above (point 1) is now resolved rather than pending.
- The BYO reverse-proxy-auth path (Authelia, Tailscale, basic auth at the proxy) is documented.

**What is left is exactly one thing: a local credential login.** av-30rj deliberately declined it — passwords drag in hashing, reset mail (so SMTP becomes config), verification, rate limiting and eventually MFA. That objection is aimed at multi-user. It mostly does not apply to this epic's case: a single credential set once at deploy has no registration, no reset flow, and no SMTP. Building it here is defensible; the scope discipline is that it must stay that narrow.

STRUCTURAL FINDING — local credential is NOT an IdentityProvider

The obvious reading of point 2 above is 'make local credential one provider implementation'. That does not fit. av-30rj's interface is redirect-based:

    AuthURL(state, verifier string) string
    Exchange(ctx, code, verifier string) (*Identity, error)

A username/password form has no redirect to an external authority and no code to exchange. Forcing it through that interface would mean inventing a fake authorization code and a self-redirect — machinery that exists only to satisfy a shape.

The correct structure is that `IdentityProvider` is one *login path*, local credential is a second, and both converge on the **same session layer**: validate, then create the same `sessions` row and set the same cookie. That is still 'one session layer instead of two', which was the actual goal of the earlier note — it just is not achieved through that particular interface. Whoever implements this should add a `/auth/local` (or extend `/auth/login` when no OIDC issuer is configured) that ends in the same session-creation call av-30rj's callback uses.

REMAINING OPEN QUESTIONS, narrowed

- Where the credential is set: env var at deploy is now clearly right for v1. A first-run setup page is friendlier but implies a writable settings surface this instance does not otherwise need, and the operator is already setting AUTH_TOKEN, EXHIBIT_SECRET and the OIDC_* vars the same way.
- Session lifetime / remember-me: av-30rj already chose an expiry for sessions rows; inherit it rather than introducing a second policy.
- CSRF: **answered, and separately ticketed as av-ke2m.** It shipped implicitly — SameSite=Lax is doing the work and is sufficient, but is neither documented nor pinned by a test. Do not re-solve it here.
