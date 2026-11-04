---
id: Exh-x16u
status: open
deps: [Exh-hwjg, Exh-zbwk]
links: []
created: 2026-11-04T05:37:11Z
type: feature
priority: 2
assignee: Max Omdal
parent: Exh-zue3
tags: [inference, experimental]
---
# Decision model provider: decide() returns typed judgments with confidence

A decision model ("System One" model, e.g. TypeSafe's Jev) answers a typed question with a judgment: yes or no, one choice from options you supply, a score on a scale you define, or a probability, with a confidence for each answer. It doesn't generate text, so its answer space is fixed by the caller, and it runs in tens to hundreds of milliseconds. Uses: routing, classification, moderation, gating, scoring.

This is a separate provider family registered with the inference core (Exh-hwjg) for the decide kind, and window.exhibit.ai.decide() on the bridge (Exh-zbwk). It shares no code with the LLM provider (Exh-gd3g) and doesn't depend on it.

Reference: Pydantic AI's decision models (https://pydantic.dev/docs/ai/models/decision/) cover the same three backends and are a good model for the question types.

## Design

- Providers: decision APIs, not the chat providers the agent uses. Start with TypeSafe (Jev) for the hosted version and a System One–compatible endpoint (/v1/systemone) for self-hosters, which also reaches local Ollama models; OpenAI Decisions after. Whether OpenRouter carries any of these is unverified; don't assume it.
- Configuration in the agent's shape but independent of it: DECISION_PROVIDER, DECISION_API_KEY, DECISION_MODEL, plus DECISION_BASE_URL for System One endpoints. Platform mode when the key is set; otherwise BYOK with a viewer-stored decision key. An instance can have decisions without an LLM and vice versa.
- window.exhibit.ai.decide({ input, instructions?, questions }), where each question is one of: boolean (with what yes and no mean), choice (options, at most the provider's cap, 255 for Jev), rating (described levels), probability, or yes/no per item of a list. Free text, unbounded numbers and dates are invalid_request before any provider call.
- Resolves to { answers, confidence, probabilities } per question. Confidence and the distribution reach the artifact unchanged; thresholds belong to the artifact's code, not to Exhibit. Values vary a few hundredths between runs, so documentation tells artifacts to threshold on ranges.
- A decision the model can't make returns unsure (kind-specific code), never a fabricated answer. No repair step and no streaming: the answer arrives whole.
- With no decision provider configured, decide() is unavailable. It never falls back to an LLM, because an artifact must never receive a confidence that means nothing.
- Metering and limits are the core's, under the decide kind; per-call defaults are small and short.

## Acceptance Criteria

- Each question type round-trips against a fake TypeSafe and a fake System One server, including confidence and probabilities.
- Unsupported question types and option counts over the cap are refused with no provider call.
- With no decision provider configured, decide() rejects with unavailable and available() omits it, even when an LLM provider is configured.
- The ledger records decide calls separately from complete calls, against the same total.
