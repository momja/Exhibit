---
id: Exh-hwjg
status: open
deps: []
links: []
created: 2026-11-04T05:47:02Z
type: feature
priority: 2
assignee: Max Omdal
parent: Exh-zue3
tags: [inference, backend, security, experimental]
---
# Inference core: payer resolution, usage metering and shared spend limits

The shared layer every artifact AI call goes through, whichever provider family serves it. It answers three questions: who pays, may they, and what did it cost. Language-model calls (Exh-gd3g) and decision-model calls (Exh-x16u) are kinds behind it, and neither reimplements any of this. Both use the agent's provider configuration; how they reach the provider is decided by Exh-urrg.

Keeping this in one place is what makes the epic's accounting rule hold: separate accounting per kind of call, one total against the limits.

## Design

- New package internal/inference. Its interface takes a call already resolved to (artifact, viewer, kind, payload) and runs it through: enablement → payer → limits → provider → metering. A provider family registers a kind (complete, decide) and implements one narrow interface: run the payload with a resolved credential and report usage and, when the provider gives one, cost.
- Payer: always the viewer, never an owner fallback. Platform mode (AGENT_API_KEY is set) charges the viewer's allowance. Otherwise the viewer's own stored agent key, and a viewer without one gets a stable no_key error. The core never accepts an anonymous or public-link principal; this is checked here as well as at the endpoint.
- Enablement: INFERENCE_ENABLED (off by default) gates everything. Each kind is available only when its model is configured and resolves for the paying provider, and the core reports which kinds are available so the bridge can say 'unavailable' rather than fail late.
- Metering: one ledger row per call as soon as usage is known, including for failed or aborted calls that reported usage. Recommendation: extend agent_usage with a nullable artifact_id and a new spend source, artifact_inference, plus the kind, rather than add a second table, so per-kind reports and the shared total are both one query. The who-pays tags (platform | user) are the agent's.
- Limits: platform-paid calls of every kind count against the same AGENT_SPEND_CAP_OWNER_CENTS and AGENT_SPEND_CAP_INSTANCE_CENTS totals as the agent, through resolveAllowance so per-owner entitlements apply. Checked before each call; a refusal names the limit and its reset. User-paid calls are never limited by the operator's defaults. Per-call belts: request-size cap, wall-clock timeout, token buckets per viewer and per artifact; each kind sets its own defaults within them.
- Guardrail: when GUARDRAIL_* is configured, platform-paid inputs are screened before the call and the screen is metered as guardrail spend, as for the agent. Each provider family declares whether it needs screening.
- Platform mode reports nothing about providers or models (av-siqf) in anything that reaches a browser.
- Startup fails if INFERENCE_ENABLED is set in platform mode with no spend cap, mirroring the agent.

## Acceptance Criteria

- With a fake provider registered for a test kind: platform and user payers are each charged correctly, and a named recipient's call is charged to the recipient.
- A platform-paid inference call and an agent run draw down one owner monthly total, and inference is reportable separately and by kind.
- Over the owner or instance limit, a call is refused before the provider is invoked.
- An anonymous or public-link principal is refused by the core even if the endpoint let it through.
- No provider or model identifier in any platform-mode result.

