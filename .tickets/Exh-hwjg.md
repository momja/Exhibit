---
id: Exh-hwjg
status: open
deps: [Exh-urrg]
links: []
created: 2026-11-04T05:47:02Z
type: feature
priority: 2
assignee: Max Omdal
parent: Exh-zue3
tags: [inference, backend, security, experimental]
---
# Inference core: payer resolution, usage metering and shared spend limits

The shared layer every artifact AI call goes through, whichever provider family serves it. It answers three questions: who pays, may they, and what did it cost. Language-model calls (Exh-gd3g) and decision-model calls (Exh-x16u) are kinds behind it, and neither reimplements any of this. Both use the agent's provider configuration. [Exh-urrg](Exh-urrg.md) selects a dedicated warm Pi Bolt registry worker, validated through shared mockLLM against the real pinned native registry/adapters. [Findings and local measurements](../docs/pi-inference-registry-spike.md) complete the architecture decision; no live provider benchmark blocks implementation. The kind tickets own actual-provider compatibility checks before enabling their integration/release.

Keeping this in one place is what makes the epic's accounting rule hold: separate accounting per kind of call, one total against the limits.

## Design

- New package internal/inference. Its interface takes a call already resolved to (artifact, viewer, kind, payload) and runs it through: enablement → payer → limits → provider → metering. A kind adapter (complete, decide) implements one narrow interface: run the validated payload with a resolved provider/model/credential through the shared worker, and report normalized usage/cost. Provider adapters/catalogs remain Pi's. Do not generalize the spike into a second service framework.
- Worker lifecycle: one inference-only Pi Bolt process, isolated HOME, no sessions/tools/discovery/MCP/startup networking; readiness handshake and bounded concurrent admission. Own its supervisor here, outside agent.Manager and agent subprocess quotas. Correlate handled-input jobs/results by ID. Every call has a fresh per-request apiKey and AbortController; never set a worker-wide runtime key or let a missing viewer key fall back to platform auth. Model-kind lookups use the catalog, not a shared credential-specific availability snapshot.
- Cancellation/failure: per-ID cancellation, wall-clock deadline, bounded grace then kill a wedged generation and fail all its in-flight calls. Restart with backoff/circuit breaking; no automatic replay of an unknown-outcome call (it may have been billed). A fresh generation serves subsequent calls. No raw provider error, request credential or RPC payload logging. The spike implements explicit replacement, not the production restart policy.
- Payer: always the viewer, never an owner fallback. Platform mode (AGENT_API_KEY is set) charges the viewer's allowance. Otherwise the viewer's own stored agent key, and a viewer without one gets a stable no_key error. The core never accepts an anonymous or public-link principal; this is checked here as well as at the endpoint.
- Enablement: INFERENCE_ENABLED (off by default) gates everything. Each kind is available only when its model is configured and resolves for the paying provider, and the core reports which kinds are available so the bridge can say 'unavailable' rather than fail late.
- Metering: one ledger row per call as soon as usage is known, including for failed or aborted calls that reported usage. Inspect stopReason (Pi can resolve with error/aborted); meter before validation/redaction. Bolt normalizes chat provider-reported cost when available; classify prices optional token usage from its pinned catalog. Preserve cost provenance; missing usage or worker death means unknown spend, not an asserted zero charge. Recommendation: extend agent_usage with a nullable artifact_id and a new spend source, artifact_inference, plus the kind, rather than add a second table, so per-kind reports and the shared total are both one query. The who-pays tags (platform | user) are the agent's.
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
- Concurrent platform/viewer calls use their own credentials; canceling one leaves the other running. A worker crash/wedge fails pending calls, starts no agent turn, consumes no agent subprocess slot, and a replacement serves a subsequent call without replaying old ones.

