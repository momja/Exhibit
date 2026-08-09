---
id: av-yisi
status: in_progress
deps: []
links: []
created: 2026-08-09T18:03:40Z
type: feature
priority: 2
assignee: Max Omdal
tags: [pi, extension, guardrails]
---
# pi extension: user prompt guardrails (guardrail LLM gatekeeper)

Create a pi extension that gates user prompts with a supplemental (small) LLM. The guardrail LLM judges whether each user prompt is acceptable: blocks/ warns on instruction-injection ('ignore previous instructions'), off-topic steering, and disallowed/explicit topics. Configurable model, policy, and mode (block/warn).

