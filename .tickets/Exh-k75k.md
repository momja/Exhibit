---
id: Exh-k75k
status: closed
deps: [Exh-v6v4, Exh-hz3g]
links: [av-e0yj, Exh-099b, av-4bzn, Exh-p4zz, Exh-nueb]
created: 2026-07-12T03:30:17Z
type: task
priority: 3
assignee: Max Omdal
parent: Exh-i0ll
---
# Extract and package the standalone exhibit-agent execution service

Updated 2026-10-20 for Exh-i0ll. Create the independently deployable
`exhibit-agent` Go supervisor/runtime, integrating only through Exh-hz3g's API
and Exh-v6v4's snapshot operations. Keep the chat UI and durable authority in
Exhibit; horizontal scaling does not require moving them to another origin.

## Design

- Extract Pi process supervision, current system prompts/extensions, scratch
  management, RPC/event adaptation, and worker-side guards/deadlines. Remove
  direct Store and in-process `agentscope.Registry` dependencies; Exhibit
  supplies the execution configuration and scoped grants over the internal API.
- Leave BYOK encryption/storage, `internal/secrets`, existing database schema
  and historical migrations, usage ledger, authorization, transcript History
  projection, browser routes/templates, and render bridges in Exhibit. Move
  shared transcript/event projection helpers to a suitable core package if
  needed so History has no dependency on the executable runtime.
- Package a reproducible release containing the Go worker, exact Pi and Node
  versions, and all extensions. Provide a standalone local invocation and
  Sprite-compatible install/bootstrap artifact; Sprites deployment does not
  assume that a Docker image is its native launch format.
- Keep per-execution HOME/scratch, minimal subprocess environment, no built-in
  tools/context discovery, and no broader credentials in Pi. Scratch and
  transient credentials are removed on close and accounted for after crashes.
- Carry forward the readiness/source-cache/concurrency changes from Exh-099b
  and bounds from av-4bzn, Exh-p4zz, and Exh-nueb as they land. Avoid recreating
  older unsafe process admission or unbounded event/transcript reads.
- Preserve mock-provider integration coverage across the real HTTP boundary.
  Move runtime test support as needed; Exhibit keeps enough test support to
  exercise browser integration without real provider keys.
- In remote mode, remove the Pi/Node requirement and local subprocess manager
  from Exhibit's runtime image. Provide explicit local-standalone versus
  Sprites-backend configuration and clear unavailable-worker behavior; no
  silent fallback that spawns Pi on the core host.

## Acceptance Criteria

1. A standalone worker creates/modifies an artifact, changes state/widgets,
   streams events, aborts, and restores a saved conversation through HTTP only.
2. Worker packages import no Exhibit Store implementation and need no core
   data volume, encryption key, operator token, or browser login cookies.
3. An Exhibit remote-mode deployment without Pi/Node serves the complete chat
   and History UI; unavailable/incompatible workers produce useful errors.
4. Current BYOK/platform, guardrail, usage, scope, conflict, snapshot, and
   bounded-memory contracts pass integration checks across the service split.
5. Runtime release/install instructions and Exhibit configuration/architecture
   documentation describe the implemented boundary and independent rollback.


## Notes

**2026-10-26T06:00:37Z**

Closed: de-scoped 2026-10-26. There is no scaling problem yet, so execution stays local. The authority layer that reached main through #189 is removed by Exh-ituu; recovery PR #215 is closed unmerged. The complete tested stack (#189-#192) survives at 3dff8a1288a3377edb586ac81777f33c2af8dfc9. If scaling becomes real, first measure per-session load on the local path and evaluate a single multi-conversation harness (Pi Durable) before reviving remote workers.
