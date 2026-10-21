---
id: Exh-xw7h
status: closed
deps: [Exh-hz3g]
links: []
created: 2026-10-20T05:25:12Z
type: task
priority: 2
assignee: Max Omdal
parent: Exh-i0ll
tags: [agent, backend, scaling]
---
# Coordinate remote agent sessions, leases, and global limits

Move the manager's process-local session ownership and admission semantics
behind durable Exhibit-side coordination so multiple standalone workers share
one set of limits. Part of Exh-i0ll; builds on Exh-hz3g's internal API.

## Design

- Persist conversation/execution/worker assignment and lifecycle transitions;
  distinguish durable conversation ID from worker ID and individual commands.
  Reserve capacity before provisioning and release reservations on failed
  startup, final close, or reconciled worker loss. No unbounded pending queue.
- Enforce the ten-open-session owner limit and configured fleet execution
  ceiling centrally, preserving idle eviction, widget-session accounting, and
  readable 429 responses. Incorporate Exh-p4zz's atomic admission work rather
  than multiplying its limit per Sprite.
- Lease the conversation to one execution with a fencing generation. Reserve
  before loading the latest snapshot/usage baseline, retain ownership through
  final persistence, and reject superseded attempts at API write boundaries.
  After a partition, an old process must not become a second effective writer.
- Route prompts, steering, abort, close, and bounded event streams to the
  assigned worker. Use stable command IDs and acknowledgements; a lost response
  is reconciled by command status, not by blindly replaying a prompt. Reconnect
  with event sequence/deduplication or explicit resynchronization on a replay
  gap so saves and terminal events are not silently missed.
- Centralize budget admission and fan out instance hard stops; maintain
  conversation lifetime spend when workers change. Define how leases expire
  while idle without keeping a Sprite awake through heartbeats or polling.
- Reconcile orphaned assignments after core/worker restart. Revoke stale tool
  grants before replacement, preserve completed writes, and expose interrupted
  runs for explicit continuation rather than automatically replaying them.

## Acceptance Criteria

1. Concurrent requests cannot exceed owner/fleet capacity or provision duplicate
   effective writers; refusing admission never starts a worker.
2. Duplicate start/prompt/close requests, simultaneous resume, delayed snapshots,
   core restart, worker loss, and network partitions have deterministic outcomes.
3. Stale workers cannot mutate artifacts or overwrite conversations; valid late
   usage is still attributed once. Replacement retains identity and spend.
4. Abort/close/hard-stop reach the correct worker, browser reconnect recovers
   state, and idle coordination permits worker compute to stop.

## Notes

**2026-10-20T07:30:55Z**

Implemented production mode routing, shared durable admission/leases, encrypted provisioning/cleanup journal, independent execution gates, idle renewal without worker HTTP, cold/full replacement restore, restart cleanup and owner erasure retries. Reviewed with review-agent and fixes re-reviewed. Real pinned-Pi/fake-Sprite concurrent-owner, admission, restart, spend continuity, guardrail/cap fanout and race regressions pass. Live platform and billing evidence remains Exh-fuxp.
