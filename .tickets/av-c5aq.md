---
id: av-c5aq
status: open
deps: [av-ep8k]
links: [av-e0yj]
created: 2026-08-05T04:49:17Z
type: feature
priority: 1
assignee: Max Omdal
parent: av-swzv
tags: [security, backend, render, multi-user]
---
# Signed render tokens: close the cross-tenant read on RENDER_ORIGIN, carry the state principal

`/a/:id` and `/w/:id` on RENDER_ORIGIN are entirely unauthenticated (internal/render/render.go ServeArtifact, ServeWidget). Anyone who knows a UUID renders the artifact — including its inlined state. With one owner the unguessable id is the only gate and that is a defensible trade; with two owners it is a cross-tenant read of both body and data.

The render surface also has no way to answer a question it will need as soon as state is principal-scoped: *whose* state should be inlined into this document.

Both are the same missing piece: the render surface has no authenticated notion of who it is serving, and deliberately cannot get one from a session.

## Design

Mint a short-lived signed token on the app origin when it builds an iframe `src` (or an "open in new tab" link), scoped to `(artifact_id, user_id)` with a few-minutes TTL. The render surface verifies the signature statelessly and uses the token's `user_id` as the state principal.

**Do not put a session cookie on RENDER_ORIGIN.** A top-level `/a/:id` is not sandboxed — it is a real-origin document with the artifact's own script inlined in it. Any cookie readable there is readable by the artifact, which can exfiltrate it to any allowlisted origin. This is why the render origin must stay sessionless and why a signed URL token is the right shape rather than an inconvenience.

The token being visible to the artifact (via `location.href` on a top-level open) is acceptable *because* it is scoped to that one artifact and short-lived: it grants only access the artifact already has to itself. That property is what makes the design safe, so keep the scope narrow — never mint an owner-wide or long-lived render token.

Same signing key can serve the agent sidecar's scoped credential (av-e0yj AC#5), which needs the same "(owner, artifact), short-lived, not the master token" shape. Worth building one minting helper rather than two.

Shares are unaffected: `/s/:shareId` stays no-auth, because the share row is the authorization (architecture.md §7). It needs no token.

Note for whoever picks this up: this ticket assumes av-ep8k has introduced the explicitly-named unscoped accessor the render path currently needs. This ticket is what removes the need for it on the `/a/:id` and `/w/:id` paths.

## Acceptance Criteria

1. `/a/:id` and `/w/:id` without a valid token do not serve another owner's artifact.
2. A token scoped to artifact A does not render artifact B.
3. An expired token is rejected.
4. No cookie is set on RENDER_ORIGIN by any route — asserted by a test, since this is the failure mode that would silently hand artifacts a session.
5. `/s/:shareId` continues to render with no token and no credentials in a fresh browser context.
6. The gallery, edit page, agent preview pane, and the `/partials/card-widget` fragment all mint tokens for the frames they embed; a gallery page of N cards does not require N round-trips to do it.
7. docs/security.md gains a short section stating why the render origin is sessionless and what the token is scoped to.

