---
id: Exh-v6v4
status: closed
deps: []
links: []
created: 2026-07-12T03:30:17Z
type: task
priority: 2
assignee: Max Omdal
parent: Exh-i0ll
---
# Persist and restore agent conversation snapshots through the HTTP API

Updated 2026-10-20 for Exh-i0ll. Replace `Session.persistTranscript`'s direct
`Store.SaveTranscript` call and provide a trusted resume-file read so a worker
can be replaced without its filesystem. This is one extraction prerequisite;
usage and spend Store dependencies are handled by Exh-hz3g.

## Design

- Add a bounded, authenticated supervisor-only snapshot write API (the original
  proposal was `PUT /api/artifacts/:id/transcripts`) and raw Pi session-file
  read for the assigned conversation. Ordinary History endpoints continue to
  return only their current safe projection, never raw files.
- Authorize using the assigned owner/artifact/conversation and execution
  attempt. The tool grant cannot reach either operation. Do not give workers
  a library-wide static token or trust an owner ID supplied in a request body.
- Retain complete Pi JSONL records, title, stable conversation ID, artifact
  version provenance, and the existing 64 MiB persistence bound. Enforce the
  byte limit before allocating the whole input; coordinate with Exh-nueb.
- Save after completed tool calls as well as settlement and final shutdown,
  sharing the persistence/drain work in av-rcwx. Distinguish a recoverable
  partial record from a settled continuation point; interrupted work is not
  automatically replayed.
- Use execution fencing and monotonic checkpoint revisions so retries are
  idempotent and late snapshots cannot replace newer history. Coordinate the
  assignment contract with Exh-xw7h. Keep version association coherent with
  the writes represented by the snapshot, rather than trusting worker text.
- Worker replacement reads the current stored snapshot only after acquiring
  the conversation assignment. Missing/foreign conversations remain a 404;
  legacy message-dump conversations remain readable but non-resumable.

## Acceptance Criteria

1. Snapshot persistence and raw resume reads work over HTTP from a separate
   worker with no Store access; browser and Pi tool credentials cannot call them.
2. Duplicate, reordered, stale-attempt, oversized, and cross-owner requests
   cannot corrupt history or publish raw session data to the browser.
3. A different worker restores the same conversation and title from durable
   history; ordinary History projections and legacy conversations still work.
4. Completed tool-call checkpoints and orderly final flush are covered, with
   interruption represented explicitly and no automatic unfinished-turn replay.

## Notes

**2026-10-20T07:30:54Z**

Implemented fenced supervisor snapshot/read APIs, monotonic checkpoint persistence, tool/settled/final acknowledgements, and raw-file restore across standalone HTTP workers. Reviewed with review-agent and fixes re-reviewed. Real pinned-Pi HTTP tests cover cold replacement, stable conversation identity, provenance, and resume; browser History remains projected. Final stack verification is documented with the integration PR.
