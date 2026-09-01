---
id: av-rcwx
status: open
deps: []
links: [av-4xao]
created: 2026-08-30T01:29:08Z
type: feature
priority: 2
assignee: Max Omdal
tags: [agent, deployment, backend]
---
# Survive a shutdown: drain on signal, persist transcripts mid-turn

A stopped or redeployed service loses agent work that had no reason to be lost.

Three gaps, found while sizing av-4xao:

1. cmd/server/main.go:251 calls http.ListenAndServe directly. No signal
   handling, no drain. Fly sends SIGINT on stop or deploy and SIGKILLs 5s
   later, so in-flight requests are cut mid-response.
2. internal/agent/agent.go:562 persists a transcript only on agent_settled. A
   turn killed before it settles leaves nothing behind, not a partial record.
   Every tool call the model already made is invisible afterward.
3. The pi subprocess is a child of the service, so it dies with the parent
   wherever it happens to be.

This is already true on every deploy. av-4xao makes it true on idle as well,
which is what raised it.

## Design

Ordered by value, and 1 is most of it.

1. Persist the transcript on tool_execution_end as well as agent_settled.
   SaveTranscript is already an upsert keyed by session, so writing more often
   costs a row rewrite and needs no schema change. A killed turn then leaves the
   record up to its last completed tool call, which is where the artifact
   writes are anyway.

2. Graceful shutdown in main: build an http.Server, signal.NotifyContext on
   SIGINT and SIGTERM, Shutdown with a timeout, and set kill_timeout in
   fly.toml above that timeout so Fly actually waits for the drain instead of
   killing through it. Both listeners in the SINGLE_LISTENER=0 path need it,
   not just the primary.

3. On shutdown, close agent sessions so each pi subprocess is signalled rather
   than orphaned, and flush transcripts before exit. Manager already owns the
   session map and a reaper (agent.go:324), so this is a Close on the same
   structure the reaper walks.

Not in scope: resuming a session across a restart. Sessions are in-memory by
design (architecture.md 3.7) and making them durable is a different decision
about where a live subprocess lives. The goal here is that a killed turn leaves
a readable record, not that it continues.

## Acceptance Criteria

A transcript exists for a session killed between tool calls, holding the calls
that completed.
SIGTERM to the server drains in-flight HTTP requests instead of cutting them,
within the configured timeout.
fly.toml kill_timeout exceeds the drain timeout.
Agent sessions are closed on shutdown, leaving no orphaned pi processes.
A test covers the mid-turn transcript write.

