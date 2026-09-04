---
id: av-q3iy
status: in_progress
deps: []
links: [av-6nbo]
created: 2026-09-04T16:01:09Z
type: feature
priority: 1
assignee: Max Omdal
tags: [hosted, security, render]
---
# Shares are framable by default; EMBED_ORIGINS becomes the lockdown

av-6nbo (PR #122) made frame-ancestors configurable for shares and shipped it opt-in. That was the wrong shape and this ticket inverts it.

The analysis in av-6nbo already said the header buys almost nothing on a share: the content is public by definition, no cookie is ever set on the render origin, every postMessage in the render preamble targets API_ORIGIN rather than '*' so a hostile framer receives nothing from the shim, and a share render holds no privileged control for a stolen click to spend. If all of that is true, denying by default is configuration for its own sake, and the cost lands on every operator who wants their own site to show their own artifacts and has to discover an environment variable first.

Concretely: the Exhibit landing page in the artifact_viewer_site repo cannot embed shares from a public instance without a production config change, which makes a marketing page a dependency of the production environment. That is backwards.

## Design

Invert the meaning of EMBED_ORIGINS. Keep the name.

- /s/:shareID with EMBED_ORIGINS unset: frame-ancestors *
- /s/:shareID with EMBED_ORIGINS set: frame-ancestors <APP_ORIGIN> <configured...>, which is now a restriction rather than a widening
- /a/:id and /w/:id: unchanged in both cases, always frame-ancestors <APP_ORIGIN> alone

The /a/:id reasoning from av-6nbo still holds and should not be touched: those routes carry a render token naming a principal and inline that principal's state, reached through a credential the app minted into a frame it controls. A share carries neither.

The code comment should say why the default is open, not only what the code does. The short version: this header is the second lock on a share, the first is the origin pinning on postMessage, and a public link that refuses to be embedded contradicts what a share is for.

EMBED_ORIGINS is not set on the Fly instance (verified), so no deployment depends on the old meaning. Even so, the docs shipped with #122 describe the opt-in reading and must be rewritten rather than amended, in deployment.md, security.md 1.8 and architecture.md 3.2.

## Acceptance Criteria

- EMBED_ORIGINS unset: a share responds with frame-ancestors *, and a page on an arbitrary origin can put that share in an iframe.
- EMBED_ORIGINS set: a share responds with frame-ancestors naming the app origin and each configured origin, and an origin outside that list is refused.
- /a/:id and /w/:id respond with frame-ancestors <APP_ORIGIN> in both cases. TestEmbedOriginsDoNotWidenTokenGatedRenders keeps passing.
- Tests cover the unset default explicitly, since that is the behaviour change.
- The three docs from #122 are rewritten to the new meaning, not patched around it.

