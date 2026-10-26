---
id: Exh-fuxp
status: closed
deps: [Exh-k75k, Exh-xw7h]
links: []
created: 2026-10-20T05:25:18Z
type: task
priority: 2
assignee: Max Omdal
parent: Exh-i0ll
tags: [agent, deployment, scaling]
---
# Run standalone agent workers on demand on Fly Sprites

Implement Exh-i0ll's production execution backend using Fly Sprites: separate
on-demand environments for concurrent conversations, with no always-running
worker floor or dependence on a large shared agent VM.

## Design

- Provision a Sprite only after central admission; install a versioned
  Exh-k75k runtime and register a Sprite Service for cold-start bootstrap.
  Provisioning uses the trusted coordinator's Sprites credential. Worker URLs
  remain authenticated; the browser talks only to Exhibit.
- Start with one conversation assignment per Sprite, preserving the same
  artifact-scoped tool authority. Do not reuse secret-bearing Sprite disks
  across owners or treat a persistent worker disk as the only conversation copy.
- Acquire/renew a bounded Tasks API hold while execution is running, release
  it after settlement/abort, and close worker event streams and idle HTTP
  keep-alives. Browser SSE may remain connected to Exhibit while the worker
  sleeps. Background health checks must not keep waking idle Sprites.
- Handle warm wake, cold service restart, and missing/deleted Sprite separately;
  reconnect and revalidate assignment before execution, or restore the latest
  central snapshot to a fresh worker. Preserve explicit interruption handling.
- Configure active-worker, startup, and idle-retention ceilings plus bounded
  provisioning retries/backoff for org concurrency and creation-rate limits.
  Show an actionable capacity error instead of an endless startup spinner.
- Reconcile failed creates, abandoned assignments, and expired Sprites; flush
  acknowledged state and revoke execution grants before normal destruction.
  Clean scratch and credentials after crashes and account deletion as well.
- Document release rollout/rollback, authenticated connectivity to Exhibit and
  the model provider, and the remaining single-writer core deployment. Record
  latency, active duration, idle behavior, worker counts, and cleanup failures
  without logging credentials or raw conversation contents.

Sprites keep disk across sleeps; compute pauses when idle. Service registration
handles cold restarts, while Tasks API holds protect active work. These are
platform mechanisms; Exhibit's central state still makes workers disposable.
References checked 2026-10-20:
[lifecycle](https://docs.fly.io/sprites/concepts/lifecycle),
[Services](https://docs.fly.io/sprites/concepts/services),
[Tasks API](https://docs.fly.io/sprites/keeping-sprites-running),
[networking](https://docs.fly.io/sprites/concepts/networking),
[pricing and org limits](https://fly.io/sprites/).

## Acceptance Criteria

1. Two real conversations run concurrently on distinct Sprites; the Exhibit
   core has no Pi subprocesses and needs no VM resize to add worker capacity.
2. An open but idle browser chat releases worker compute, and a later prompt
   succeeds after warm wake, cold restart, and complete Sprite replacement.
3. Forced worker loss, failed provisioning, capacity refusal, and interrupted
   requests preserve acknowledged state and never silently duplicate a run.
4. Idle expiration/orphan reconciliation deletes leftover Sprites, revokes
   credentials, and removes owner scratch data without losing durable history.
5. Rollout evidence records measured cold/warm readiness, concurrency and
   memory bounds, idle compute release, retained-disk cost, actual org limits,
   and working rollback. Cost verification uses observed usage rather than a
   promise that sleeping persistent storage is free.

## Notes

**2026-10-26T06:00:37Z**

Closed: de-scoped 2026-10-26. There is no scaling problem yet, so execution stays local. The authority layer that reached main through #189 is removed by Exh-ituu; recovery PR #215 is closed unmerged. The complete tested stack (#189-#192) survives at 3dff8a1288a3377edb586ac81777f33c2af8dfc9. If scaling becomes real, first measure per-session load on the local path and evaluate a single multi-conversation harness (Pi Durable) before reviving remote workers.
