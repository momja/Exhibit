---
id: Exh-gd3g
status: open
deps: [Exh-hwjg, Exh-zbwk, Exh-urrg]
links: []
created: 2026-11-04T05:36:51Z
type: feature
priority: 2
assignee: Max Omdal
parent: Exh-zue3
tags: [inference, agent, backend, experimental]
---
# Language model provider: complete() through the agent's provider configuration

The language-model half of artifact inference: a provider family registered with the inference core (Exh-hwjg) for the complete kind, and window.exhibit.ai.complete() on the bridge (Exh-zbwk).

It uses the same provider names and the same two modes as the agent (internal/agent/agent.go providerEnv). [Exh-urrg](Exh-urrg.md) selects registry.complete() in the dedicated warm Pi Bolt worker owned by the core. [Findings](../docs/pi-inference-registry-spike.md) include the runnable Go prototype and deterministic shared mockLLM measurements through the real pinned registry/adapters. The architecture spike is complete; implement against mock providers without a live benchmark prerequisite. Do not start duplicate Go provider clients.

## Design

- A narrow complete kind adapter under internal/inference: system text, messages, optional JSON Schema, max output tokens and resolved model in; text or validated JSON plus normalized usage/cost out. Dispatch through the core's worker to registry.complete(), with a per-call credential/signal. Pi owns provider protocol adapters; this kind still maps schema options to provider-native structured output and validates/repairs output. OpenRouter is first/hosted default; verify Anthropic, OpenAI, Google and OpenCode Go against their configured catalog models. Use shared mockLLM for inference repository tests too, preserving the real registry/adapters in round-trip coverage (the spike redirects provider HTTP transport; the existing agent exhibit-mock registration alone does not prove this path). Never route through an agent conversation.
- Configuration follows the agent: platform mode (AGENT_API_KEY + AGENT_PROVIDER) uses the instance credential, otherwise BYOK uses the viewer's stored agent key and provider. INFERENCE_MODEL chooses the model and is required in platform mode; BYOK has a per-provider code default.
- Cost comes from Pi's normalized usage: catalog estimates, overridden by valid provider accounting where the pinned Bolt adapter supports it (including OpenRouter chat). Preserve provider-versus-estimate provenance and usage on error/aborted results before mapping errors. Do not add a parallel price table. Unknown usage is not a zero-cost assertion; a genuinely unpriced model may retain exact tokens with a zero estimate, as for the agent.
- window.exhibit.ai.complete({ messages, schema?, maxTokens? }) resolves to text, or to parsed JSON validated against schema when one is given. Invalid structured output is invalid_output after at most one repair attempt; unvalidated output never reaches the artifact.
- Per-call defaults within the core's belts: a clamped max output and a timeout suited to prose.
- Compatibility: if Claude's artifact runtime still exposes window.claude.complete(prompt), map it onto complete() so AI-powered artifacts pasted from Claude run unchanged. Verify the current contract first; drop this item if it no longer exists.

## Acceptance Criteria

- Platform and BYOK calls each reach the configured provider, tested against the mock provider.
- complete() with a schema returns validated JSON or a stable error.
- Startup fails if INFERENCE_ENABLED is set in platform mode without INFERENCE_MODEL.
- Browser test: an approved artifact's complete() round-trips through the host to the mock provider.
- Use the shared worker cancellation/lifecycle, with no per-call Pi startup, proven deterministically through shared mockLLM and the real pinned registry/adapters.
- Before enabling an actual provider integration/release, run an opt-in/manual registry.complete() compatibility check against its configured provider/model, including structured output and usage/error behavior. This is separate release validation, not a live benchmark or implementation gate for the completed architecture spike; mock success makes no claim about live compatibility, accuracy or spend.
