---
id: av-6nbo
status: closed
deps: []
links: [av-q3iy]
created: 2026-09-04T05:45:55Z
type: feature
priority: 2
assignee: Max Omdal
tags: [hosted, security, render]
---
# Let configured origins embed render documents

buildCSP hardcodes `frame-ancestors " + appOrigin`, so nothing but the app origin can embed a render document. A share is public by definition, yet a site other than the app cannot put one in an iframe.

Verified against the live instance: framing https://artifacts.dizzard.net/s/<id> from another origin is refused by the browser, and the frame renders the broken-document icon.

Wanted by artifact_viewer_site, the landing page, which is meant to show real artifacts from a public instance rather than local copies of them.

## Design

Add an EMBED_ORIGINS config value, empty by default. buildCSP takes the extra origins and emits `frame-ancestors <appOrigin> <extras...>`. Unset, the emitted policy must be byte-identical to today.

Decide whether the widening applies to /s/:shareID only or to /a/:id as well. My read: keep /a/:id tight. It inlines the owner's state and carries a principal in the render token. A share carries neither and is already public, so it is the one worth loosening.

Security reasoning worth writing into the code comment, because it is easy to overstate what this header is doing here. The real defence against a hostile framer is that every postMessage in the render preamble targets API_ORIGIN rather than '*', so a page on another origin receives nothing from the shim. No cookie is ever set on the render origin either, so there is no session to clickjack, and a share render holds no privileged control to trick a click into. frame-ancestors is a second lock, not the first one. That is why widening it for shares costs little, and also why it should stay opt-in rather than become the default.

## Acceptance Criteria

- EMBED_ORIGINS unset: the policy string is unchanged and the existing render tests pass untouched.
- EMBED_ORIGINS set: frame-ancestors names the app origin followed by each configured origin.
- A share loads inside an iframe served from a configured origin.
- A share is still refused from an origin that is not configured.
- The /a/:id decision is made deliberately and the reason is in the code, not just the commit.

Out of scope, found while investigating: buildCSP emits no frame-src and no child-src, so with default-src 'none' an artifact cannot embed any iframe at all. That blocks an artifact from ever containing another artifact. File separately if that is wanted.

