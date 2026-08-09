---
id: av-30rj
status: in_progress
deps: [av-ep8k]
links: [av-q30x, av-ke2m, av-5imk, av-wmp6, av-c5aq, av-ep8k, av-syug, av-g2dx]
created: 2026-08-05T04:50:17Z
type: feature
priority: 2
assignee: Max Omdal
parent: av-swzv
tags: [backend, auth, multi-user, design]
---
# Pluggable identity provider seam: keep OSS vendor-free, allow a hosted tier

Goal: the open-source project stays independent of any auth vendor and self-hostable with no identity server at all, while a hosted deployment can run a real multi-user IdP (Clerk is the immediate candidate) without a fork.

This is filed as a **decide-then-implement** ticket, in the shape of av-wrbu: one open question below changes the acceptance criteria materially and is a business-posture call, not an engineering one.

Relationship to av-q30x: that epic is single-user login (credential + session cookie, `owner_id` stays 1) and explicitly excludes OIDC. This ticket is the seam that lets a provider be swapped in behind the same session layer. If av-q30x has not shipped yet, there is an argument for doing the session layer once, here, and making local-credential login one provider rather than the foundation — see "Interaction with av-q30x".

## Design

## The standard shape

OIDC via discovery, exchanged **once** at login for our own session cookie. The IdP is a login-time concern only:

```
Browser -> our session cookie --+
                                +--> owner_id
API/CLI -> our bearer token ----+

The IdP appears exactly once, at /auth/callback.
```

Per-request JWKS verification is the API-token pattern and is the wrong default for a server-rendered app: it puts a network check on the request path and makes logout impossible (a signed token stays valid until TTL regardless of what the IdP thinks). Owning the session fixes both. This is what Grafana, Gitea, Outline, and Immich all do — built-in default auth, optional OIDC behind a few env vars.

## The seam

Because the exchange happens once, the vendor surface collapses to roughly:

```go
type IdentityProvider interface {
    AuthURL(state, verifier string) string
    Exchange(ctx context.Context, code, verifier string) (*Identity, error)
}

type Identity struct{ ExternalID, Email string }
```

Everything downstream — cookies, sessions, `owner_id`, the render token (av-c5aq), the agent credential — is ours and identical across providers. Swapping Authentik for Clerk touches one constructor.

## Concrete

- Config: `OIDC_ISSUER`, `OIDC_CLIENT_ID`, `OIDC_CLIENT_SECRET`. Use the issuer's `/.well-known/openid-configuration` for discovery rather than hand-configured endpoints — that is what makes "any IdP" true in practice rather than in principle.
- Flow: Authorization Code + PKCE.
- Tables: `users(id, external_id, email, created_at)`, `sessions(id, user_id, expires_at)`. Opaque random session id in the cookie, looked up per request — more standard than a JWT cookie precisely because it is revocable.
- **Store `email` on the users row, not just `external_id`.** Provider subjects are provider-specific; switching providers orphans every user unless there is a re-link key. Cheap now, painful to retrofit.
- Cookie: HttpOnly, Secure, SameSite=Lax, app origin only. Never on RENDER_ORIGIN (av-c5aq explains why).
- Routes: `/auth/login`, `/auth/callback`, `/auth/logout`.
- Default stays single-user/static-token. An unset `OIDC_ISSUER` changes nothing about any current deployment.
- Library: `coreos/go-oidc/v3` + `golang.org/x/oauth2` — the conventional Go pairing, and the full code exchange needs more than a JWKS-only verifier.

## Provider fit

- Authentik / Keycloak / Zitadel / Dex / Ory: config only. Best fit for the OSS ethos — self-hosted, redirect-based login (zero frontend work, no third-party JS, no CDN, which matters given the project's standing self-host-everything rule for htmx and Phosphor).
- Clerk / Auth0 / Okta / WorkOS: config plus optional lifecycle webhooks. Verify Clerk's generic-OIDC surface before assuming config-only — their mainstream path is their own SDK, and if it does not fit, the hosted module implements the two-method interface against that SDK instead. Either way it is contained, which is the point of putting the seam at the callback.
- Better Auth: a TypeScript library, not an IdP. Would mean a Node sidecar beside the Go service, which breaks the one-binary-one-process deployment promise. Accommodated by the generic verifier if someone wants to run it; not designed for.

## Deliberately not building

Local password auth as a first-class feature. It looks like the neutral choice and is not: password hashing, reset emails (so now SMTP is a config requirement), verification, rate limiting, eventually MFA. Delegate to an IdP or stay single-user — those two cover every real self-hoster. (av-q30x reaches the opposite conclusion for a single-user instance where the credential is set at deploy and no reset flow is needed; that is defensible in isolation and the two should be reconciled deliberately.)

## OPEN QUESTION — blocks the acceptance criteria

Where does the hosted layer live? This decides whether the OSS repo needs restructuring.

**(a) Nested module at `hosted/`** (`module github.com/momja/Exhibit/hosted`). Go's `internal/` visibility is import-path-prefix based, not module based, so a module under the parent path *can* import `github.com/momja/Exhibit/internal/api`. Root `go.mod` stays vendor-free, no packages need promoting. Catch: a nested module in a public repo is public.

**(b) Separate private repo** importing the OSS module. Requires exposing a composition API (a root `exhibit` package building the server from a Config including an `IdentityProvider`) and promoting `store`/`blob`/`api` types out of `internal/`. More restructuring, but the hosted code stays private.

Pick (a) if hosted-specific code can be public; (b) if it cannot.

## Interaction with av-q30x

If av-q30x has not started: consider building the session layer here once (cookie + `sessions` table + middleware), with local-credential login as one `IdentityProvider` implementation rather than the foundation. That gives one session layer instead of two and makes av-q30x's v1 a thin provider. If av-q30x has already shipped, this ticket adds a provider behind its existing session layer and changes nothing user-visible for single-user instances.

## Acceptance Criteria

Blocked on the open question above. Provisional:

1. With no OIDC config set, behaviour is byte-identical to today's single-user instance (static token, owner 1) — asserted by the existing suite passing unchanged.
2. With `OIDC_ISSUER` set, a user can complete Authorization Code + PKCE against a generic OIDC provider and land authenticated, with a `users` row created just-in-time on first login.
3. Session is our own cookie; logout revokes it server-side and the revoked session is rejected immediately (not at token TTL).
4. `owner_id` remains an integer; no table outside `users` references a provider-specific identifier.
5. No vendor SDK appears in the root `go.mod`.
6. A second provider can be configured without changes outside its constructor — demonstrated by a test double implementing `IdentityProvider`.
7. Docs state the BYO reverse-proxy-auth path (Authelia, Tailscale, basic auth at the proxy) as a supported alternative, consistent with the "TLS/proxy is the operator's" stance.

