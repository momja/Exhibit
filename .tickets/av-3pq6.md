---
id: av-3pq6
status: open
deps: [av-3tib]
links: [exhibit-lwb]
created: 2026-07-06T22:01:58Z
type: task
priority: 2
assignee: Max Omdal
---
# Support artifact source code version history

Keep prior versions of an artifact's source body instead of destructively overwriting it. A new version is created by either write path that replaces the body: a save from the CodeMirror edit page, and a refetch from `source_url` (POST /api/artifacts/:id/refetch). Both already flow through the single write path, so versioning is a snapshot step inside those handlers before the blob is replaced.

## Design

- **Stack, not tree — restore is one-directional.** Restoring V3 while at V5 does not rewind: it pushes a *copy* of V3 as the new head (V6). V4/V5 remain in history; nothing is ever rewritten or discarded. The UI must make this significance clear at the restore action (e.g. "Restore as new version" phrasing, not "Revert").
- **Storage:** an `artifact_versions` table (artifact_id, seq, blob_id, created_at, origin: edit|refetch|restore) behind the Store interface; bodies stay in the Blob store like the head body.
- **Retention: keep all versions.** Matches the store-forever library thesis. Two mitigations for the exhibit-lwb interplay (vendored snapshot-on-import bodies can be multi-MB, bounded by lwb.2's total-bytes cap): (1) skip version creation when the incoming body hash equals the current head — an unchanged refetch costs nothing; (2) store version blobs content-addressed by hash so identical bodies share storage. A per-artifact version-bytes budget can come later if it ever hurts in practice.
- **UI: list + view + restore** on the detail/edit page. Each entry shows origin (edit/refetch/restore), timestamp, and a thumbnail — depends on av-3tib so version snapshots can carry a thumbnail.
- **PRD clarification (in scope):** §8.1's "no version history" refers to externally *vendored* snapshots and is not wrong, but it should be reworded to distinguish external vendoring from the internal version stack this ticket introduces.

## Acceptance Criteria

- Editor save and refetch each snapshot the prior body as a new version; an unchanged refetch (same content hash) creates no version.
- Restore pushes the selected version as the new head; intermediate versions remain listed; the UI labels the action as creating a new version.
- Version list on the detail/edit page shows origin + timestamp (+ thumbnail once av-3tib lands), with view and restore per entry.
- Deleting an artifact deletes its versions and their blobs (extends the existing delete path).
- PRD §8.1 updated to distinguish internal version history from one-time external vendoring.


## Complexity

L
