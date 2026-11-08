---
id: Exh-zbwk
status: open
deps: [Exh-hwjg]
links: []
created: 2026-11-04T05:36:51Z
type: feature
priority: 2
assignee: Max Omdal
parent: Exh-zue3
tags: [inference, render, security, ui, experimental]
---
# Inference bridge: window.exhibit.ai in the render preamble, endpoint and approvals

The browser half shared by every kind of call: how artifact code reaches the inference core (Exh-hwjg), and who may cause a call. It carries a kind and a payload and knows nothing about either; the provider tickets (Exh-gd3g, Exh-x16u) add their methods on top.

Artifact code calls a method on window.exhibit.ai. The preamble posts the call to the host frame (pinned to APP_ORIGIN, like the storage shim). The host makes the authenticated request to POST /api/artifacts/:id/inference and posts the result back into the frame, where the call's promise settles. The frame gets no endpoint, no credential and no connect-src source.

## Design

- Preamble: a bridge alongside download/clipboard/links in bridgeScript (internal/render/render.go). Framed-only. Absent from widgets, anonymous and public-link renders, and the top-level render, which has no host frame. There, window.exhibit.ai is still defined but its methods reject with 'unavailable', so artifact code can degrade instead of throwing on undefined. window.exhibit.ai.available() reports which kinds this instance and this frame can use.
- Endpoint POST /api/artifacts/:id/inference with { kind, payload }: resolves the artifact with GetArtifactReadableBy (owner or named grantee). Public-mode, anonymous and agent credentials are refused; the agent has its own model. Passes the viewer to the core as payer.
- One capability for every kind: the person is asked whether this tool may use AI, never which kind of model.
  - Artifact-level: a new inference_approved column in store.ApprovalColumns, set only by the owner through the ordinary first-use prompt in app chrome. It names the artifact and says that text it sends, possibly including saved data, goes to an AI service. Revocable from the artifact's security settings and shown in the capability cluster.
  - Viewer-level: because a recipient spends their own allowance, each viewer also consents once per artifact, in the same shape as relay consent. For the owner it is one prompt covering both. A recipient of an approved artifact is asked only about their own spend; a recipient of an unapproved one gets the refusal explained in chrome with no button (av-awr4).
- Version preview: allowed for the owner and metered.
- Errors reaching the artifact are one stable set shared by every kind (unavailable, not_approved, no_key, limit_reached, invalid_request, rate_limited, provider_error, plus kind-specific codes each provider ticket names), never upstream text.

## Acceptance Criteria

- Browser test with a fake kind: a call round-trips through the host and the core and resolves in the frame.
- An unapproved artifact raises the owner's first-use prompt; Allow persists inference_approved and retries the call.
- A named recipient's call is charged to the recipient; the owner's ledger is unchanged.
- Public link, anonymous, widget and top-level renders cannot cause a call. Tested at the endpoint, not only by absence of the bridge.
- security.md §4 and PRD §6.3 describe the capability and its two consents.
