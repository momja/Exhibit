---
id: av-5imk
status: closed
deps: []
links: [av-wmp6, av-rgp1, av-q30x, av-30rj, av-c5aq, av-ep8k, av-syug]
created: 2026-08-06T16:02:07Z
type: bug
priority: 1
assignee: Max Omdal
tags: [security, public-mode, frontend, gallery]
---
# Public gallery pages would hand the operator's AUTH_TOKEN to anonymous visitors

Not a live bug today — a trap laid directly in the path of av-eu3v, av-epnt and av-n8v5, which are the next tickets anyone would pick up.

The gallery *page* routes (`/`, `/artifacts/:id`, `/artifacts/:id/edit`, `/agent`, `/new`) sit **outside** the authenticated API group and always have — they are HTML, and the page's own JS authenticates its API calls with a bearer token that `gallery.go` and `agentui.go` embed into an inline bootstrap `<script>`. That token is `AUTH_TOKEN`: the operator's full-authority service credential.

That was safe while every page visitor *was* the operator. av-wmp6 changed the surrounding conditions: a public instance now serves anonymous readers, and `publicVisitor(ctx)` marks them. Nothing marks a *page* request as a public visitor yet — correctly, because without an identity provider the operator and a visitor are indistinguishable on those routes — so no page is anonymous-readable and nothing leaks.

The moment a public gallery page renders for an anonymous visitor, the bootstrap script hands them `AUTH_TOKEN`. That is not a read of a public library; it is full write authority over every artifact, every collection, the share table and the BYO provider key — recoverable only by rotating the secret.


## Notes

**2026-08-06T16:02:07Z**

DESIGN

The fix is not 'remember to strip the token'. Anything a future page author must remember will eventually be forgotten, and the failure is silent — the page works perfectly while leaking.

Make it structural, in roughly this order:

1. **Never emit a credential the viewer is not entitled to.** The bootstrap should carry a token derived from the *request*, not the process. An anonymous page render emits **no** token at all, and the page JS treats its absence as read-only rather than erroring. A session-authenticated render can rely on the cookie the browser already sends and emit no token either — av-30rj made the session a real credential, so the embedded bearer token is arguably vestigial for browser clients generally, not just public ones.

2. **Pin it with a test that cannot be forgotten.** Assert no page-route response body ever contains `AUTH_TOKEN`'s value when the request is anonymous. Walk the page routes the way `csrf_test.go` (av-ke2m) walks the API mux and `renderheaders_test.go` (av-nr0p) walks the render mux — both fail loudly on an undeclared route, which is the property that matters here. A new page route must not be able to ship untested.

3. **Only then** build the public pages. av-eu3v / av-epnt / av-n8v5 should depend on this rather than each solving it, because three independent solutions is three chances to get it wrong.

Worth noting while here: this is the same class of finding as av-rgp1 (the API bearer token in the SSE URL query string). Both are 'the operator's full-authority credential travelling somewhere it need not go'. If av-rgp1's fix introduces a narrower, request-scoped browser credential, that is very likely the same mechanism this ticket wants — check it before inventing a second one.

Also worth checking: `rendertoken.Signer` (av-c5aq) already mints short-lived, narrowly-scoped credentials and av-wmp6 extended it with an anonymous claim that subtracts authority. The shape needed here — 'a browser credential that is not the service token' — may be a third caller of that helper rather than a new mechanism.

**2026-08-09T04:05:59Z**

ESCALATION (2026-08-08): this is a live defect in shipped auth, not only a public-mode trap

The original description frames this as a trap awaiting the public-page tickets. It is broader than that, and it undermines the property av-30rj was built to deliver.

Every page render passes `ro.cfg.AuthToken` **unconditionally** — gallery.go:105/118/146/182 and agentui.go:141 — with no reference to how the visitor authenticated. So on an instance with OIDC correctly configured and `sessionGate` correctly gating the pages:

1. The user logs in through the provider and receives a session cookie.
2. They load any page. That page hands their browser `AUTH_TOKEN` — the operator's full-authority service credential — in an inline bootstrap script.
3. They log out. The session row is deleted, so the cookie is dead.
4. **The service token is not.** It is in the page source they already loaded, in devtools, in anything that copied it. It grants full API authority over every artifact, every collection, the share table and the BYO provider key, and it cannot be revoked for one person — only rotated for everyone.

So logout does not revoke API access. That is precisely the property av-30rj chose an opaque server-side session for in the first place ('a row can be deleted, so logout revokes immediately instead of at some TTL the server cannot influence' — 013_users_sessions.sql). The session layer keeps its promise; the page bootstrap breaks it.

This raises the priority: it is not gated on the public-page work, and a multi-user or hosted deployment is unsafe until it is fixed. It should land with — or before — the epic reaches main.

The fix does not change: emit a credential derived from the request, not from the process. For a session-authenticated browser the correct answer is likely **no token at all** — the cookie is already sent automatically with same-origin fetches, so the embedded bearer token is vestigial for browser clients generally, not just anonymous ones. Confirm the page JS paths work cookie-only before removing it (SSE via EventSource is the one to check — see av-rgp1, which is the same credential in a different wrong place).
