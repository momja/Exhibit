---
id: Exh-hz3g
status: open
deps: []
links: []
created: 2026-07-12T18:03:33Z
type: task
priority: 2
assignee: Max Omdal
parent: Exh-i0ll
---
# Define the Exhibit core / standalone worker API and authority boundary

Updated 2026-10-20 for Exh-i0ll. Keep the chat UI, login, browser-facing agent
routes, and renderer bridges on their current origins. Replace the old
`AGENT_URL` link-out/additional-embedder plan with an authenticated backend
boundary for standalone execution. The worker must have no Store dependency.

## Design

- Introduce an execution-backend interface for create/attach, prompt/steer,
  abort/close, event delivery, and readiness. Define a versioned internal HTTP
  contract with command identity, execution identity, bounded payloads,
  deadlines, explicit errors, and capability/version negotiation sufficient
  for safe independent rollout. Exh-xw7h implements assignment and routing.
- Keep encrypted BYOK data, `EXHIBIT_SECRET`, platform key selection, login,
  and policy authority in Exhibit. Resolve the authenticated owner's run
  configuration centrally; send only credentials needed by that execution
  through the trusted channel. Never expose them through page bootstrap,
  event URLs, command-line arguments, or logs.
- Exhibit issues/binds/revokes the per-artifact tool grant; include execution
  identity so reassigned workers lose write authority. A supervisor credential
  may report usage or persist its assigned transcript but must not become Pi's
  credential. Unbound create grants bind exactly once server-side.
- Replace direct `RecordAgentUsage`, `SessionAgentSpend`, `OwnerAgentSpend`,
  and `InstanceAgentSpend` dependencies with trusted reporting and central
  admission/budget decisions. Stable usage identities make reporting retries
  idempotent; preserve attribution, compaction/tool/guardrail spend, resumed
  baselines, and valid late reports from a terminated attempt. Raw per-worker
  counters are not the fleet-wide budget authority.
- Preserve platform model-identity filtering, guardrail refusal behavior,
  owner/session soft-cap semantics, instance hard-stop propagation, and turn
  deadlines. Define fail-closed behavior when policy/usage state is unavailable
  and a bounded reporting retry policy.
- Coordinate snapshot operations with Exh-v6v4 and assignment/fencing with
  Exh-xw7h. Retain owner-checked browser APIs and existing cookie/single-use
  SSE ticket behavior. Page changes continue through htmx template partials.

## Acceptance Criteria

1. The internal API can drive a separate worker without shared memory or a
   database mount; unknown/incompatible protocol versions fail clearly.
2. Owner/artifact/execution authorization is checked centrally. Tool credentials
   cannot reach supervisor routes, and no infrastructure or operator token is
   inherited by Pi or returned to a browser.
3. Usage retries, resume baselines, policy outage, cross-owner requests, and
   stale execution grants are covered; budgets have one central authority.
4. The current UI and renderer bridges require no new origin, public worker
   endpoint, browser-held worker token, or CORS allowance.

