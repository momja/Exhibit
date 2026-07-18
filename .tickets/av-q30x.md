---
id: av-q30x
status: open
deps: []
links: [av-q3wo, Exh-mety]
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


## Complexity

XL
