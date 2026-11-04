---
id: Exh-urrg
status: open
deps: []
links: []
created: 2026-11-04T15:56:51Z
type: task
priority: 2
assignee: Max Omdal
parent: Exh-zue3
tags: [inference, agent, spike, experimental]
---
# Spike: run artifact inference through Pi's model registry instead of new Go provider clients

Both kinds of artifact call can probably be served by code that already exists. Pi's model registry has provider adapters for every provider the agent uses, a catalog that prices usage, registry.complete() for chat models, and registry.classify() for classifier (decision) models, a uniform contract over typesafe-system-one and the other classifier APIs. The usage-policy guardrail (internal/agent/ext/guard.ts) already uses classify() with Jev served through OpenRouter.

Writing Go clients instead would duplicate all of that: the adapters, the classify protocol and the price table that gives the ledger its cost column. But Pi runs today as one subprocess per agent conversation, which is far too heavy to start for one call from an artifact. This spike decides the runtime before Exh-gd3g and Exh-x16u are built.

## Design

Questions to answer, with a short written recommendation:

- Can a long-lived Pi Bolt process host a small inference extension that the Go server calls per request (RPC from host to extension, concurrent requests, cancellation), using the pinned Pi version? If not, what is the smallest shim that can (e.g. a long-lived worker using the same registry)?
- Per-request credentials: the guardrail passes apiKey as a per-request option. Confirm that one worker can serve platform and BYOK calls concurrently without credentials leaking between requests.
- Latency and memory of one warm worker against a direct HTTPS call, for a decide() call in particular, where the point is speed.
- Usage and cost: confirm complete() and classify() results both carry usage with catalog cost, for OpenRouter at least.
- Failure model: a crashed or wedged worker must fail calls closed and restart without taking the server down; the agent's subprocess limits must not count it.
- Fallback: if the worker is not viable, the Go client per kind that Exh-gd3g and Exh-x16u describe, and what is lost (cost estimates, classify contract).

Record the decision in the epic and update Exh-hwjg, Exh-gd3g and Exh-x16u to match.

## Acceptance Criteria

- A prototype answering a complete() and a classify() call through OpenRouter from Go, with measured latency.
- A written decision with the trade-offs above, linked from the epic.

