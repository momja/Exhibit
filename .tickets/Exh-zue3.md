---
id: Exh-zue3
status: open
deps: []
links: []
created: 2026-11-04T05:36:21Z
type: epic
priority: 2
assignee: Max Omdal
tags: [inference, agent, security, experimental]
---
# Platform inference for artifacts: LLM and decision model calls without managing keys

An artifact can call a language model, or a decision model, from ordinary JavaScript. The person running it doesn't need a key, a provider account or any API knowledge. Exhibit makes the call on the server and charges it to the person viewing the artifact.

This is a capability bridge (PRD §6.3), not a connector. The frame never reaches the network. The render preamble installs window.exhibit.ai, the call is posted to the host frame like a state write, and the host makes the authenticated request to the single write path. The CSP and the artifact's origin allowlist are untouched.

Product decisions (2026-11-04):

- Two kinds of call, each its own ticket. complete() is a general LLM call. decide() calls a decision model (a "System One" model such as TypeSafe's Jev): it answers typed questions (yes/no, choice, rating, probability) with a confidence, and never generates text.
- One shared core and one bridge: who pays, whether they may, metering and limits live in one layer (Exh-hwjg), and the person is asked once whether a tool may use AI, never which kind of model.
- No inference on public links or for anonymous visitors, ever. No owner-sponsored links.
- Who pays: the viewer. On an artifact shared with named people, each recipient's calls spend that recipient's own allowance (or their own key), never the owner's.
- Providers follow the agent pattern: platform mode uses an instance credential, self-hosters may configure any supported provider, and BYOK uses the viewer's own stored key. Both kinds use the agent's providers (OpenRouter on the hosted version, which serves Jev as the guardrail already does): INFERENCE_MODEL names a chat model and INFERENCE_DECISION_MODEL a classifier model. The provisional runtime decision is one dedicated warm Pi Bolt registry worker, subject to the live-call acceptance gate below.
- Accounting is separate from the agent's (its own ledger tag, reported separately), but it counts toward the same total usage limits.

Out of scope here: streaming, scheduled inference into saved state for widgets, tool use or multi-step agents inside artifacts, and in-browser local models.

## Runtime decision — 2026-11-05 (provisional)

[Exh-urrg](Exh-urrg.md) recommends the existing checksum-pinned Pi Bolt 1.0.8 / Pi 1.4.0 executable with a small inference-only extension, handled-input RPC dispatch, per-request keys and cancellation, and a Go supervisor owned by the inference core. Reuse registry.complete() / registry.classify(); do not implement parallel Go clients unless the live gate invalidates this choice. Worker admission/restart is independent of agent conversation limits. Full evidence, trade-offs, prototype and reproduction commands: [Pi inference registry spike](../docs/pi-inference-registry-spike.md).

Real-Bolt local-fixture checks passed concurrency, credential isolation, cancellation, cost normalization and crash/wedge replacement. Warm classifier median was 2.834 ms versus direct Go HTTP 1.616 ms (20 local samples each); warm worker RSS was 91,940 KiB. These are not real provider/HTTPS measurements.

The spike stays in progress: no authorized OpenRouter credential was available, so neither live inference acceptance nor provider latency is established. Also confirm the pinned /api/v1/systemone classifier transport against the current catalog's /api/alpha/decisions endpoint. Keep dependent runtime implementation gated on Exh-urrg; a public model catalog and unauthenticated 401 probes do not satisfy that gate.

