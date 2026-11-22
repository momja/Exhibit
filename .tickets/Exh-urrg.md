---
id: Exh-urrg
status: closed
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
- Local overhead and memory of one warm worker against a direct mock-provider HTTP call, for a decide() call in particular, where the point is speed.
- Usage and cost: confirm complete() and classify() results both carry usage with catalog cost, for OpenRouter at least.
- Failure model: a crashed or wedged worker must fail calls closed and restart without taking the server down; the agent's subprocess limits must not count it.
- Fallback: if the worker is not viable, the Go client per kind that Exh-gd3g and Exh-x16u describe, and what is lost (cost estimates, classify contract).

Record the decision in the epic and update Exh-hwjg, Exh-gd3g and Exh-x16u to match.

## Acceptance Criteria

- A prototype answering complete() and classify() from Go through the real pinned Pi registry/adapters against shared mockLLM, with measured local worker/direct overhead and memory.
- Deterministic concurrency, per-request credentials, cancellation, usage/error accounting and crash/wedge recovery checks; no conversation per call. Live provider benchmarks are not a completion gate.
- A written decision with the trade-offs above, linked from the epic.

## Completed result — mockLLM scope clarification, 2026-11-19

The user clarified: “Why do we need openrouter benchmarks? We should be able to test with mockLLM?” This architecture spike uses deterministic provider HTTP responses; live compatibility is separate opt-in/manual release validation in the kind tickets. No credentials, paid calls or live benchmarks are required for spike completion or dependent implementation.

[Findings, pinned primary-source citations and commands](../docs/pi-inference-registry-spike.md), [Go prototype](../scripts/pi-inference-spike/main.go), [extension](../scripts/pi-inference-spike/extension.ts), [shared mockLLM](../internal/mockllm/inference.go), and [local measurements](../scripts/pi-inference-spike/results/mockllm-bolt-1.0.8-arm64.json).

Decision: one independent, warm, checksum-pinned Pi Bolt 1.0.8 / Pi 1.4.0 worker, an inference-only handled-input RPC extension, and Go supervision in the inference core. Reuse complete/classify adapters/catalogs; narrow Go adapters remain the fallback if later integration or production budgets invalidate the choice. Epic, core, both kinds and ZIP prerequisite are aligned. The prototype uses explicit fresh-worker replacement; production supervisor/backoff/circuit breaking belongs to the core.

Shared mockLLM supplies both HTTP protocols and deterministic usage/error responses. The real native registry, model lookup, adapters and catalog remain in the path. Race tests pass twelve overlapping mixed calls with zero credential mismatches, cancellation of each kind without sibling harm, bounded admission, malformed-answer usage preservation, crash/wedge failure of pending calls and successful explicit fresh-worker replacement. Selected existing agent create/edit and guardrail pipeline regressions pass with the pinned binary.

Twenty warm samples per path/kind: complete median 3.816 ms worker vs 1.638 ms direct; classify 2.533 ms vs 1.583 ms. Ready 135.777 ms (one observation); warm worker RSS 94,804 KiB. These are local HTTP transport/accounting checks, not provider/HTTPS latency, model accuracy or real spend. Benchmark defaults to mockLLM; live mode requires explicit -live.

Provider release validation remains in [complete](Exh-gd3g.md) and [decide](Exh-x16u.md): the pinned classifier route /api/v1/systemone versus the catalog-advertised /api/alpha/decisions is unverified, not proven incompatible by unauthenticated 401s. Do not enable an unverified actual-provider integration or patch transport speculatively. This does not block implementation.
