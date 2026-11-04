---
id: Exh-gd3g
status: open
deps: []
links: []
created: 2026-11-04T05:36:51Z
type: feature
priority: 2
assignee: Max Omdal
parent: Exh-zue3
tags: [inference, agent, backend, experimental]
---
# Inference service: provider client, usage metering and shared spend limits

The server-side half of platform inference. It takes a request already resolved to (artifact, viewer, kind), decides which credential pays, calls the provider, records the usage and enforces the limits. It has no knowledge of frames or bridges; the endpoint and the preamble are separate tickets.

There is no Go model client today: every model call goes through a Pi subprocess. A per-session subprocess is far too heavy for a single structured call, so this adds one, under the same provider names the agent supports (internal/agent/agent.go providerEnv).

## Design

- New package internal/inference with a small Provider interface: one request (system text, messages, an optional JSON Schema, max output tokens, model) in; one response (text or JSON, usage, provider-reported cost when present) out. One adapter per provider in providerEnv, each using that provider's native structured-output mode. OpenRouter is the first and the hosted default; Anthropic, OpenAI, Google and OpenCode Go follow the same interface. exhibit-mock / MOCK_LLM_URL works here too, so tests never reach a provider.
- Configuration follows the agent's two modes, chosen by the same variable. Platform mode (AGENT_API_KEY + AGENT_PROVIDER) uses the instance credential. Otherwise it is BYOK: the call uses the viewer's stored agent key and provider (agent_keys), and a viewer with no key gets a stable error the host chrome can explain. INFERENCE_ENABLED (off by default) gates the whole feature. INFERENCE_MODEL and INFERENCE_DECISION_MODEL choose the two models; both are required in platform mode. In BYOK mode each provider has a code default for each kind.
- Platform mode reports nothing about the model, as the agent doesn't (av-siqf): no provider or model name in any response that reaches a browser.
- Metering: every call writes a ledger row as soon as usage is known, including for failed or aborted calls that reported usage. Same who-pays tags as the agent (platform | user), and a new spend source, artifact_inference, with the artifact id. Recommendation: extend agent_usage with a nullable artifact_id rather than a second table, so totals stay one sum. Cost comes from the provider when it reports one (OpenRouter does); otherwise from a price table. Zero cost with exact tokens is acceptable, as it is for the agent.
- Limits: platform-paid inference counts against the same AGENT_SPEND_CAP_OWNER_CENTS and AGENT_SPEND_CAP_INSTANCE_CENTS totals as the agent, read through resolveAllowance so per-owner entitlements apply. Checked before each call, refused with a message naming the limit and its reset, as the agent's between-runs check does. A BYOK call is never limited by the operator's defaults. Per-call bounds are the inference-specific belt: a clamped max output tokens, a request-size cap, a wall-clock timeout, and token buckets per viewer and per artifact.
- Guardrail: when GUARDRAIL_* is configured, screen platform-paid prompts before the call (they protect the same provider account) and meter the screen as guardrail spend, as the agent does.
- Ownership of the call: the viewer is the payer. The service takes the viewer principal and never an owner fallback; the endpoint ticket decides who may be a viewer.

## Acceptance Criteria

- Platform and BYOK calls each charge the right payer, tested against the mock provider.
- A platform-paid inference call and an agent run draw down one owner monthly total; both sources are reportable separately.
- Over the owner or instance limit, a call is refused before any provider request, and the refusal names the limit and reset.
- Structured calls return JSON that validates against the request schema or a stable error; the provider's raw output never reaches the caller unvalidated.
- No provider or model identifier in any platform-mode response.
- Startup fails if INFERENCE_ENABLED is set in platform mode without both model variables or without a spend cap, mirroring the agent's no-cap refusal.

