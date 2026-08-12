---
id: av-nr0p
status: in_progress
deps: []
links: [av-c5aq]
created: 2026-08-06T04:59:18Z
type: bug
priority: 2
assignee: Max Omdal
tags: [security, render, csp]
---
# Render document sets no Referrer-Policy, so the render token can reach third-party logs

av-c5aq puts a short-lived credential in the render URL as a query parameter (`?t=`). The render surface sets `Content-Security-Policy`, `Content-Type` and `Cache-Control` on that document (internal/render/render.go, the three `Header().Set` calls around line 188) and no `Referrer-Policy`.

A token in a URL is precisely the case that header exists for. Current browsers default to `strict-origin-when-cross-origin`, which strips path and query from cross-origin referrers, so this is mostly covered in practice today — but it is a browser default doing security work this codebase otherwise does explicitly, and the artifact can opt itself back out of it.

Found while reviewing av-c5aq after merge; not a regression, a gap in it.


## Notes

**2026-08-06T04:59:40Z**

DESIGN

Set `Referrer-Policy: no-referrer` on every response the render surface emits, alongside the CSP and Cache-Control it already sets. The render document has no legitimate need to attribute its own outbound requests, and `no-referrer` is the only value that holds regardless of what the artifact's own markup asks for.

Why the accidental case is the one that matters. The malicious artifact is already answered by scope and TTL: the token names one artifact, expires in ten minutes, is read-only (the render origin registers three routes, all GET), and is accepted at exactly one Verify call site — so a hostile artifact that exfiltrates it gains almost nothing it could not already do by sending the inlined state directly. The case this ticket closes has no attacker in it at all: an honest artifact loads a font or image from an allowlisted CDN, and the render URL — token included — lands in that third party's access logs.

Two paths the default does not cover:

1. An artifact controls its own <head> and can set `<meta name="referrer" content="unsafe-url">`, opting itself back into sending the full URL to every allowlisted origin. A response header wins over the document's meta, so setting it server-side is what makes this not the artifact's choice.
2. The browser default has changed over time and is not uniform across engines. Relying on it leaves a security property outside our control on a surface where every other one (CSP, sandbox, no-store) is explicit.

Scope note: this is a header, not a redesign. The token stays in the URL — that is deliberate and av-c5aq explains why a cookie on RENDER_ORIGIN would be far worse (a top-level /a/:id is a real-origin document with the artifact's own script inlined, so any cookie readable there is readable by the artifact).

Worth checking in the same pass whether the app origin's `/artifacts/:id/open` redirect should carry it too — the Location header it emits contains a freshly minted token, and the browser's history entry for the destination will hold it either way.

**2026-08-06T04:59:40Z**

ACCEPTANCE CRITERIA

1. Every render-surface response — /a/:id, /w/:id, /s/:shareId — carries `Referrer-Policy: no-referrer`.
2. A test asserts the header on all three routes. Prefer a table over three separate tests, so a fourth render route added later fails loudly rather than shipping bare.
3. The header is present on error responses too (404 on a rejected token), not only on the success path — a rejected render still has the token in its URL.
4. The existing av-c5aq assertion that no render route emits Set-Cookie continues to pass; this ticket adds a header, it does not touch that one.
5. docs/security.md records why a credential-bearing URL gets no-referrer, in the section av-c5aq added about the render origin being sessionless.
