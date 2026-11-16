---
id: Exh-lmor
status: closed
deps: []
links: [Exh-38vu]
created: 2026-11-16T06:42:36Z
type: chore
priority: 2
assignee: Max Omdal
tags: [security, cleanup, public-mode, docs]
---
# Retire PUBLIC_MODE_ENABLED: a flag with no reachable consumer

`PUBLIC_MODE_ENABLED` is shipped, documented and tested, and it publishes
nothing anybody can see. It should be removed rather than finished: the demo
gallery it was meant to become is a separate site built from artifact exports
(`GET /api/artifacts/:id/export`, av-vnkt), which needs no flag, no anonymous
page route and no moderation surface in the core.

## Why it is dead, precisely

- **The page half was never built.** `sessionGate` admits anonymously only
  `/auth/`, `/assets/`, `/api/`, `/s/`, `/manifest.json`, `/favicon`
  (`publicPathPrefixes`), so on any instance with a login an anonymous `GET /`
  or `/artifacts/{id}` is a 302 to `/auth/login`. Pinned today by
  `TestAnonymousPublicVisitorStillResolvesThePublicOwner`
  (`internal/api/pageowner_test.go`), and stated in `pagecredential.go`: the
  anonymous page branch "is dormant; it exists so that the ticket which opens a
  page to anonymous readers inherits the answer."
- **The API half cannot be reached from a browser either.** `/api/*` sends no
  `Access-Control-Allow-Origin` (`internal/api/oauth.go` is the only route that
  does), so no other origin can read the library with JS, and same-origin JS
  only exists on pages the gate blocks. The one remaining consumer is `curl`.
- **The payload has no consumer.** `GET /api/settings/public` is served and
  tested but nothing in `web/gallery/` or any template fetches it, so
  `PUBLIC_INSTANCE_NAME` and `PUBLIC_INSTANCE_DESCRIPTION` render nowhere.
- **Four tickets around it are closed without code:** av-eu3v, av-epnt,
  av-n8v5, av-ra31. Leaving the flag in place keeps advertising a feature the
  repo does not have, to every future reader and agent.
- **It is not free.** `publicReadable` is a deny-by-default allowlist every
  future `/api/artifacts/*` sub-resource has to be reasoned against, and it
  widens the anonymous-read argument in `security.md`.

## Scope

This ticket deletes the flag. The sentence below is **not a second task**: it is
the escape hatch, so nobody has to re-derive the smaller option if a server-side
reader turns up in review and argues for keeping the anonymous API. Delete the
API too, and that argument never arises.

If that does happen, freeze instead of deleting: keep items 4 and 5 below, delete
everything else, retire the name/description vars and the settings route, and
correct the docs and the four tickets.

**Code to delete**

1. `internal/api/publicmode.go` — `PublicMode`, `PublicModeFromEnv`,
   `publicSettings`, `publicSettingsResponse`, the `envPublic*` constants and
   `envOwnerID`. Keep `envBool`: `hostdispatch.go` (`SingleListenerFromEnv`)
   depends on it, so move it to a neutral file.
2. `ro.Get("/api/settings/public", ro.publicSettings)` in `setupRoutes`.
3. `Config.Public` (`internal/api/api.go`) and the `publicMode :=
   api.PublicModeFromEnv()` wiring in `cmd/server/main.go`.
4. `authMiddleware.publicRead` and `publicReadable` (`middleware.go`). Keep
   `artifactsSubPath` — `agentScopeAllows` shares it.
5. `PrincipalPublic` (`principal.go`), `publicVisitor`, and every reader:
   `pagecredential.go` (`anonymous`, `pageToken`), `gallery.go` (`renderURLs.
   anonymous`, `navigationWithoutCredential`), `gallerygrid.go`, the
   `PrincipalPublic` case in `admin.go`.
6. The anonymous render path, if nothing else sets it: `renderURLs.mint`'s
   `MintAnonymous` branch and `rendertoken.Signer.MintAnonymous`/
   `MintAnonymousFor` with their tests, plus the renderer's anonymous narrowing
   in `render.go`/`injectPreamble`. Confirm with `grep -rn MintAnonymous` and
   `grep -rn 'rd.Anonymous\|viewer.Anonymous'` before deleting the renderer
   side; the claims field may stay as a no-op if a test still names it.
7. Tests that exist only for this: `publicmode_test.go` (all but the env-parsing
   cases, if `envBool` survives), the public case in `pageowner_test.go`,
   `pagecredential_test.go`, `navigation_test.go`, `admin_test.go`, and the
   `cfg.Public = ...` fixtures in `galleryspaces_test.go`, `oauth_test.go`,
   `agentgrants_test.go`, `agentgrants_panel_test.go`.

**Docs to correct**

- `docs/deployment.md`: the four `PUBLIC_*` rows (§1 table) and the whole
  "public instance" paragraph at §1 including "What turning it on actually
  opens" and "What a visitor does not get"; note that a public demo is a
  separate site built from exports.
- `docs/architecture.md` §(config) references and the av-wmp6 paragraph at
  line ~161.
- `docs/security.md`: the allowed-principal table (line ~418, ~484), the "The
  optional `a` claim renders for nobody" line (~187), and the two public-mode
  claims at ~282 and ~957.

**Tickets to annotate (not delete — the history is worth keeping)**

- Add a note to av-ec0t, av-4ac9, av-wmp6, av-7oyy, av-eu3v, av-epnt, av-n8v5,
  av-ra31 stating that public mode was retired in this ticket rather than
  completed, and that the gallery is a separate site.
- **One coordination check, not a dependency.** Exh-fkla is in progress and its
  acceptance criteria mention filtering drafts out of public-mode lists. Nothing
  needs to happen *to* that ticket: with no public principal, drafts stay private
  because reads are owner-scoped, not because a public filter excludes them.
  Re-read its ACs after the deletion and, if the wording now describes something
  that no longer exists, note it there and move on.

## Acceptance criteria

1. `rg 'PUBLIC_MODE|PUBLIC_INSTANCE|PUBLIC_OWNER|PrincipalPublic|publicVisitor|publicReadable|settings/public'` over `internal/`, `cmd/`, `web/`, `docs/`, `README.md` returns nothing.
2. An unauthenticated `GET /api/artifacts` returns 401 on every configuration (previously 200 with the flag on).
3. `GET /api/settings/public` is not routed (404 like any unknown path).
4. No unreachable branches left behind: `go vet ./...`, `make test`, and `make lint` pass, and no function is left uncalled because its only caller was public mode.
5. `docs/` no longer describes a feature that does not exist, and states where the public demo lives instead.
6. The eight tickets above carry the retirement note; Exh-fkla still meets its own ACs without a public principal.


## Notes

**2026-11-16T07:14:44Z**

Deleted the flag/API bypass instead of using the freeze escape hatch: no server-side reader was identified. Preserved envBool in env.go and artifactsSubPath for agentScopeAllows. Legacy anonymous render claim is rejected (including signed tokens) instead of silently becoming an owner render. Auth-disabled no-token/no-login pass-through remains the existing separate policy; 401 regression covers token-only and login-enabled configurations, including retired env values. This is a literal every-configuration AC limitation for independent review, not a retained public-mode API.

**2026-11-16T23:01:14Z**

Recovery merged origin/main and retained ticket history, including restoring this task file after an unrelated main commit deleted it. Removed the obsolete public-mode operator-panel test case. Isolated grant revocation/credential refresh/retirement regressions pass; prior package timeout occurred during SQLite fixture migration at cumulative deadline, not a proven per-test deadlock. Build and go vet pass. golangci-lint unavailable. Auth-disabled compatibility remains unchanged and needs explicit acceptance of the literal every-configuration AC exception. Parent owns fresh independent final-head review and all merge decisions; this implementer does not merge.

**2026-11-16T23:09:07Z**

CI identified one remaining obsolete public-only OAuth fixture after cfg.Public removal. Removed that fixture only; authenticated bearer rejection, cross-origin rejection, agent refusal and existing no-login operator approval/exchange tests are preserved. Focused TestOAuthConsentRequiresPersonAndSameOrigin passes after the fix. Previous broad local short run was stopped when this concrete CI failure arrived and is incomplete, not a pass. Final-head review must be renewed after this test cleanup.
