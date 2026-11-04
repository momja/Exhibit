---
id: Exh-gd3g
status: open
deps: [Exh-hwjg, Exh-zbwk]
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

There is no Go model client today: every model call goes through a Pi subprocess, which is far too heavy for one call from an artifact. This adds one, under the same provider names and the same two modes the agent uses (internal/agent/agent.go providerEnv).

## Design

- An LLM provider interface under internal/inference: system text, messages, an optional JSON Schema for structured output, max output tokens and model in; text or JSON, usage and provider-reported cost out. One adapter per provider in providerEnv, each using that provider's native structured-output mode. OpenRouter is the first and the hosted default; Anthropic, OpenAI, Google and OpenCode Go follow. exhibit-mock / MOCK_LLM_URL works here too.
- Configuration follows the agent: platform mode (AGENT_API_KEY + AGENT_PROVIDER) uses the instance credential, otherwise BYOK uses the viewer's stored agent key and provider. INFERENCE_MODEL chooses the model and is required in platform mode; BYOK has a per-provider code default.
- Cost comes from the provider when it reports one (OpenRouter does), otherwise from a price table. Zero cost with exact tokens is acceptable, as it is for the agent.
- window.exhibit.ai.complete({ messages, schema?, maxTokens? }) resolves to text, or to parsed JSON validated against schema when one is given. Invalid structured output is invalid_output after at most one repair attempt; unvalidated output never reaches the artifact.
- Per-call defaults within the core's belts: a clamped max output and a timeout suited to prose.
- Compatibility: if Claude's artifact runtime still exposes window.claude.complete(prompt), map it onto complete() so AI-powered artifacts pasted from Claude run unchanged. Verify the current contract first; drop this item if it no longer exists.

## Acceptance Criteria

- Platform and BYOK calls each reach the configured provider, tested against the mock provider.
- complete() with a schema returns validated JSON or a stable error.
- Startup fails if INFERENCE_ENABLED is set in platform mode without INFERENCE_MODEL.
- Browser test: an approved artifact's complete() round-trips through the host to the mock provider.
