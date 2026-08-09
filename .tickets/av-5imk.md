---
id: av-5imk
status: closed
deps: []
links: [av-wmp6, av-rgp1]
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
