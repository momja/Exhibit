---
id: Exh-pd9t
status: open
deps: []
links: []
created: 2026-10-18T23:03:59Z
type: feature
priority: 2
assignee: Max Omdal
parent: Exh-c2hu
tags: [versions, state, history]
---
# Store lossless state deltas with compressed full snapshots and bounded replay

Every superseded version currently stores the complete owner state map as state_json. A growing dataset is repeatedly copied even when a checkpoint changes only a small part of it.

## Design

Use immutable state snapshot identities, unchanged-state reuse, compressed full baselines and lossless deltas. Account for large JSON/string datasets held under one localStorage key: changed-key replacement alone is insufficient. Preserve exact string bytes, additions, deletions and empty values; choose full encoding if a delta is larger, bound replay by chain depth and accumulated bytes. Keep the live per-key table and exact transaction boundary. Existing full state_json snapshots remain readable or migrate atomically. Delta dependencies stay retained through compaction/deletion and owner scoping remains enforced.

## Acceptance Criteria

Restore/preview reproduce exact maps across arbitrary strings, Unicode, large single-key arrays, deletes, clear, unchanged saves and restore-as-new-version. Existing databases upgrade safely; encoding/version/hash errors fail without partial restore. Benchmark CPU, peak memory, physical bytes and worst-case reconstruction for growing, append-heavy and replacement-heavy state. Bound replay with measured thresholds. Do not promise faster saves merely from smaller storage.

