---
id: av-1rvm
status: open
deps: []
links: [av-y98v, av-p4hm]
created: 2026-08-03T03:51:10Z
type: feature
priority: 2
assignee: Max Omdal
tags: [draft, state]
---
# Artifact state durability: undo, snapshots, and export

Artifact state has no undo and no way out of the service. Both gaps got sharper with av-p4hm, which deliberately made state destruction WORK: clear() now really wipes every row (av-st7c), the edit page has an 'Erase all data' button (av-hg5f), and the agent sidecar can call delete_state with the key omitted to erase everything (av-lvi1). Before that epic, state was accidentally hard to destroy because the destruction paths were buggy. Now there are three working irreversible paths and zero recovery behind any of them — one of which an LLM can invoke from a chat message.

av-hg5f's own ticket records the gap as an accepted note ('destructive and irreversible — no version history for state'). That was defensible when the erase path barely worked. It is weaker now.

Separately, state is the one part of an artifact the user cannot take with them. The PRD's thesis (§1, architecture §1.1) is that an artifact is a file you own and can walk away with; the body honours that and the state does not.

SHAPE IS NOT DECIDED — this is a draft. See the open questions.

Supersedes the unbuilt half of av-y98v (download an archive of an artifact's stored data); av-y98v's other half, displaying the data on the edit page, shipped as av-hg5f.

## Design

Open questions, deliberately unresolved:

1. ONE CONCERN OR TWO? Undo-before-destruction (server-side safety buffer) and user-facing export/import (portability) share a serialization format and little else. They may be one ticket or two. Portability is the stronger product argument; undo is the stronger safety argument.

2. SNAPSHOT SHAPE. Note the inversion against the live store: artifact_state is one row per (artifact,key) precisely so cross-device writes get per-key last-write-wins and concurrent writes to different keys don't clobber. An archive wants the opposite — a single atomic blob, because a restore should return the artifact to a coherent moment rather than merging fragments. Rows for the live path, blob for the archive.

3. TRIGGER. Snapshot before every destructive op (clear, erase-all, agent delete_state)? On a schedule? On demand only? Automatic-before-destructive is the cheapest thing that closes the safety gap.

4. RETENTION. How many, how long, and where — SQLite rows or the blob store? Note artifact bodies already live behind the Blob interface (architecture §3.3) and Blob has no Delete in v1, so retention needs thought before choosing that home.

5. FORMAT. Whatever a user downloads is a compatibility surface once it exists. A flat JSON object of the localStorage view is the obvious candidate and matches what GET /api/artifacts/:id/state already returns.

6. IMPORT/RESTORE. Read-only export is much cheaper than round-tripping. Restore reopens the merge question in 2 and needs a single write path (architecture §4.1) like everything else.

7. SCOPE. localStorage-backed state only, presumably. sessionStorage is frame-local and never persisted after av-9jll, so there is nothing to archive.

## Acceptance Criteria

To be defined once the shape is settled. Blocking question before any implementation: is this the undo/safety ticket, the portability/export ticket, or both?

