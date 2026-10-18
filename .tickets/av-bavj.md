---
id: av-bavj
status: closed
deps: []
links: [av-3pq6, av-1rvm, av-reo3, av-20fk, av-5s7g, av-y7td, av-b4yh]
created: 2026-10-03T06:00:00Z
type: feature
priority: 2
assignee: Max Omdal
tags: [versions, state]
---
# Artifact version history: every change is a version, with the state it left behind

Every change to an artifact's body or widget — a manual save, an agent write, a refetch, a widget save or removal, a restore — is recorded as a version, so any earlier state can be returned to. A version is more than a diff of text: it carries the saved data (`localStorage` state) the code had written by the time the next version replaced it, so a restore puts back a pair that belongs together.

Supersedes [[av-3pq6]] (source history) and the undo half of [[av-1rvm]] (state), and is the recoverability [[av-reo3]] asked for. Part of a stack: [[av-5s7g]], [[av-y7td]] and [[av-b4yh]] build on it.

## Design

- `artifact_versions` (migration 031): one row per version, never rewritten except `state_json` (set once, when superseded). The head is a version; `artifacts.source_blob_id` / `widget_blob_id` mirror it and are written only by `store.CommitVersion` / `RestoreVersion`, in the same transaction. Version 1 exists for every artifact by an `AFTER INSERT` trigger (the migration backfills existing ones).
- Blobs are never overwritten: a change writes a new blob, so an older version's bytes are exactly what they were. The deletion queue's refcount and the `blob_references` view count version rows, so history is charged to its owner and goes with the artifact.
- State is snapshotted right before every change, in the same transaction: the owner's `artifact_state` rows as one JSON object on the version being replaced. Exact, and no caller can forget it. Stored in SQLite, not the blob store — it is bounded, atomic with the version insert, and cascades with the artifact.
- Restore pushes a copy as a new version (never a rewind), replaces the live state with the snapshot that version left behind, and snapshots the state it replaces first, so it can itself be undone. Restoring the head is a 409.
- Not versioned: allowlist, capability approvals, shares, tags, title. A restore never widens what the artifact may reach; the existing `footprint_changed` gate re-reviews a restored body's origins.
- Provenance: `origin` (initial|edit|agent|refetch|restore), `message` (an agent's prompt, a refetch's URL), `session_id`. An agent write reads the last two off its grant (`agentscope.Grant`), which the server holds.
- Owner-only. `versions` is absent from `agentSubResources`: restoring is a person's decision.
- Edit page: a Versions panel (server-rendered list, Restore with a confirmation that says what is kept).

## Acceptance Criteria

- Manual edits, agent writes, refetches and widget saves/removals each create a version; a save that changes nothing creates none.
- State is snapshotted in the same transaction as the change, onto the version it replaces.
- Restore returns code, widget and saved data as a new version and can itself be undone.
- Deleting an artifact or account removes every version's blobs; storage accounting counts them.
- Another owner's artifact answers 404 on both routes; an agent credential is refused.
- PRD §8.1 distinguishes version history from one-time vendoring; §8.5 describes the flow.

## Notes

Found while building: a URL ingest stores the page with an injected `<base href>` but refetch stores the raw page (pinned by `TestRefetchArtifactOverwritesBody`), so the first refetch of every URL-ingested artifact differs from what ingest stored and records a version. Left as is — it is an existing inconsistency, not part of this change — but it is worth its own ticket.

Retention is keep-everything. A prune policy, a running preview of an old version, and a version diff are follow-ups.
