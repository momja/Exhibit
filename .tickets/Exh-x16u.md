---
id: Exh-x16u
status: open
deps: [Exh-zbwk]
links: []
created: 2026-11-04T05:37:11Z
type: feature
priority: 2
assignee: Max Omdal
parent: Exh-zue3
tags: [inference, experimental]
---
# Decision model: window.exhibit.ai.decide() always returns validated structured output

A decision model is a fast, cheap 'system one' model whose answers are always structured: classify this, route that, score these, extract those fields. decide() is its own call so an artifact never has to remember to ask for structure, and so the operator can point it at a model chosen for latency and price rather than depth.

## Design

- window.exhibit.ai.decide({ input, schema, instructions? }): schema is required; a call without one rejects before leaving the frame. Resolves to the parsed, validated object, never a string.
- Runs on INFERENCE_DECISION_MODEL (Exh-gd3g). The server validates the response against the schema and makes at most one repair attempt before returning invalid_output; the artifact never sees unvalidated output.
- Schema limits: a bounded size and depth, and the subset of JSON Schema every supported provider's structured-output mode accepts. Anything else is invalid_schema at the endpoint, with the offending keyword named.
- Tighter per-call defaults than complete() (small max output, short timeout), metered and limited exactly like complete() under the same artifact_inference source, with the kind recorded so decide and complete can be reported apart.
- Same bridge, endpoint, approvals and payer rules as Exh-zbwk; no new consent.

## Acceptance Criteria

- decide() without a schema, or with an unsupported keyword, is refused with a stable error and no provider call.
- A mock provider returning malformed output yields one repair attempt and then invalid_output.
- A valid call resolves to an object matching the schema, tested across at least the OpenRouter and one other adapter.
- The ledger distinguishes decide from complete within artifact_inference.

