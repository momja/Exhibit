---
id: Exh-x16u
status: open
deps: [Exh-hwjg, Exh-zbwk, Exh-urrg]
links: []
created: 2026-11-04T05:37:11Z
type: feature
priority: 2
assignee: Max Omdal
parent: Exh-zue3
tags: [inference, experimental]
---
# Decision model: decide() returns typed judgments with confidence

A decision model ("System One" model, e.g. TypeSafe's Jev) answers typed questions with a judgment rather than text: yes or no with a probability, one choice from options you supply with a distribution and a confidence, or a score on a scale you define with a confidence. Its answer space is fixed by the caller, and it is fast enough to sit inside an interaction. Uses: routing, classification, moderation, gating, scoring.

This registers the decide kind with the inference core (Exh-hwjg) and adds window.exhibit.ai.decide() on the bridge (Exh-zbwk). It doesn't depend on the LLM half (Exh-gd3g); either can ship first.

Precedent: the usage-policy guardrail (internal/agent/ext/guard.ts, av-gust) already asks Jev one bool question through Pi's registry.classify(), served by OpenRouter. decide() exposes the same contract to artifacts.

## Design

- Providers: the agent's provider pattern, no separate decision configuration. Platform mode uses the instance credential (AGENT_API_KEY + AGENT_PROVIDER; OpenRouter on the hosted version, which serves Jev). BYOK uses the viewer's stored key and provider. INFERENCE_DECISION_MODEL names a classifier model in Pi's catalog for that provider; startup fails in platform mode if it is unset or doesn't resolve to a classifier. In BYOK mode, a provider with no classifier model makes decide() unavailable for that viewer.
- Runtime: [Exh-urrg](Exh-urrg.md) provisionally selects registry.classify() in the core's dedicated warm Pi Bolt 1.0.8 / Pi 1.4.0 worker. [Findings and prototype](../docs/pi-inference-registry-spike.md) prove local operation/isolation but not live OpenRouter acceptance. Keep this dependency unresolved until authenticated classification verifies the pinned /api/v1/systemone transport (current OpenRouter catalog advertises /api/alpha/decisions). Do not silently use a new endpoint or claim the pinned adapter passed. If this fails, revise the spike decision before adding a narrow Go fallback.
- window.exhibit.ai.decide({ state, questions }) mirrors Pi's ClassifierContext. state is a JSON object (the thing being judged). Each named question is one of:
  - bool: instructions, criteria { true, false } → { probability }
  - choice: instructions, criteria { option: meaning } → { choice, probabilities, confidence }
  - score: instructions, criteria [level descriptions] → { score, confidence }
  Anything else is invalid_request before any provider call. Bounds on option count, level count, question count and state size, within the provider's own caps.
- Answers use Pi's normalized shape above, probabilities/confidence included where specified; this is not a passthrough of all provider wire fields (Pi's score normalization omits extra score probabilities). Validate finite/range-correct values and choice membership; Pi's parser alone does not enforce all these bounds. Thresholds belong to the artifact's code, not to Exhibit. Answers vary slightly between runs, so the docs tell artifacts to threshold on ranges.
- No text generation, no repair step and no streaming: the answer arrives whole or the call fails (provider_error).
- With no classifier model available, decide() is unavailable and available() omits it. It never falls back to a chat model, because an artifact must never receive a probability that means nothing.
- Metering and limits are the core's, under the decide kind, using optional usage priced from Pi's pinned catalog, explicitly an estimate. Meter usage before rejecting error/aborted results, including malformed billed answers; missing usage is unknown spend, not zero. Per-call defaults are small and short; send per-request cancellation through the shared worker, never session-wide abort.

## Acceptance Criteria

- Each question type round-trips against a fake classifier provider, including probabilities and confidence.
- Unsupported question types and over-limit inputs are refused with no provider call.
- With no classifier model for the provider, decide() rejects with unavailable even when complete() works.
- The ledger records decide calls separately from complete calls, against the same total.
- One end-to-end check against OpenRouter's Jev, run manually or behind an opt-in test flag.
