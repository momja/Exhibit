---
id: av-4bzn
status: closed
deps: []
links: [av-wrbu, Exh-w3h0, Exh-p4zz, Exh-nueb]
created: 2026-08-03T05:02:38Z
type: bug
priority: 2
assignee: Max Omdal
tags: [security, agent, dos]
---
# Bound agent subprocess output and retained SSE events

Pi stdout is read with ReadBytes, which allocates without a line limit. The replay backlog retains 4096 events without a byte budget, and slow SSE subscribers each retain up to 1024 arbitrarily large events. Bound these parts of the event path together.

## Design

- Read stdout with bounded storage; discard oversized lines through the next newline and log a warning without publishing partial JSON. Continue with the next valid event.
- Keep both count and byte limits on the replay backlog, releasing references to evicted events.
- Bound retained event bytes and subscriber count so slow clients cannot defeat the session memory ceiling.
- Preserve event order and existing SSE behavior for events within the limits.

## Acceptance Criteria

1. An oversized or unterminated stdout line is consumed with bounded storage and never dispatched as a partial event; subsequent valid lines still work.
2. Replay has both count and byte ceilings, with evicted event references released.
3. Slow SSE clients and excessive subscriptions cannot grow retained event memory without bound.
4. Regression tests cover line boundaries, recovery, replay eviction, and slow subscriber behavior.

## Scope split

The service-wide body limit, explicit HTTP timeouts, ten-session owner cap, 429 mapping, and transcript persistence ceiling are already implemented. Remaining independent defects are tracked in Exh-w3h0 (prompt/image validation), Exh-p4zz (atomic process admission and instance ceiling), and Exh-nueb (bounded transcript reads). Those are outside this fix.

## Notes

**2026-10-19T19:54:04Z**

Implemented bounded JSONL reading (8 MiB per line), replay and subscriber queues (4096 events / 16 MiB each), four subscribers per session, explicit queue cleanup, and ordered draining at SSE shutdown. Published bytes are copied so trimmed reader buffers cannot evade accounting. Oversized lines are discarded through newline with a warning; slow subscribers drop oldest queued events. Added boundary, recovery, eviction, ownership, subscription-limit, shutdown, and cleanup regression coverage. Independent prompt, admission, and transcript fixes remain in the three split tickets.

**2026-10-19T19:57:27Z**

Validation: make assets; full go test ./internal/agent; full go test -race ./internal/agent; targeted API session ownership and 429 tests all pass. Updated integration helpers to consume asynchronous replay explicitly.

**2026-10-19T19:58:03Z**

The full internal/api suite also passed (403.739s). The earlier combined invocation reported failure only because its agent package ran before the asynchronous-replay test helper correction; the corrected full agent suite and full agent race suite subsequently passed.
