---
id: Exh-zbwk
status: open
deps: [Exh-gd3g]
links: []
created: 2026-11-04T05:36:51Z
type: feature
priority: 2
assignee: Max Omdal
parent: Exh-zue3
tags: [inference, render, security, ui, experimental]
---
# Inference bridge: window.exhibit.ai in the render preamble, endpoint and first-use approval

The browser half: how artifact code reaches the inference service, and who may cause a call.

Artifact code calls window.exhibit.ai.complete({ messages, schema?, maxTokens? }). The preamble posts the call to the host frame (pinned to APP_ORIGIN, like the storage shim). The host page makes the authenticated request to POST /api/artifacts/:id/inference, and posts the result back into the frame, where the call's promise settles. The frame gets no endpoint, no credential and no connect-src source.

## Design

- Preamble: a bridge alongside download/clipboard/links in bridgeScript (internal/render/render.go). Framed-only. Absent from widgets, anonymous and public-link renders, and the top-level render, which has no host frame. There, window.exhibit.ai is still defined but rejects with a stable 'unavailable here' error, so artifact code can degrade instead of throwing on undefined.
- Endpoint POST /api/artifacts/:id/inference: resolves the artifact with GetArtifactReadableBy (owner or named grantee). A public-mode or anonymous principal is refused. An agent credential is refused too; the agent has its own model. Calls the inference service with the viewer as payer.
- Artifact-level approval: a new inference_approved column in store.ApprovalColumns, set only by the owner through the ordinary first-use prompt in app chrome. It names the artifact and says that text it sends, possibly including saved data, goes to an AI service. Revocable from the artifact's security settings and shown in the capability cluster.
- Viewer-level consent: because a recipient spends their own allowance, each viewer also consents once per artifact, in the same shape as relay consent. For the owner it is one prompt covering both. A recipient of an artifact the owner approved gets a prompt that asks only about their own spend. A recipient of one the owner did not approve gets the refusal explained in chrome, with no button, per av-awr4.
- Version preview: allowed for the owner and metered; the call's result isn't saved by anything except the artifact's own state writes, which preview already discards.
- Errors reaching the artifact are a small stable set (unavailable, not_approved, limit_reached, invalid_schema, provider_error, rate_limited), never upstream text.
- Compatibility: if Claude's artifact runtime still exposes window.claude.complete(prompt), map it onto complete() so AI-powered artifacts pasted from Claude run unchanged. Verify the current contract before implementing; drop this item if it no longer exists.

## Acceptance Criteria

- Browser test: an approved artifact's complete() round-trips through the host to the mock provider and resolves in the frame.
- An unapproved artifact raises the owner's first-use prompt; Allow persists inference_approved and retries the call.
- A named recipient's call is charged to the recipient; the owner's ledger is unchanged.
- Public link, anonymous, widget and top-level renders cannot cause a call. Tested at the endpoint, not just by absence of the bridge.
- Security docs (security.md §4 taxonomy, PRD §6.3) describe the new capability and its two consents.

