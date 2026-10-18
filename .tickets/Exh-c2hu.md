---
id: Exh-c2hu
status: open
deps: []
links: []
created: 2026-10-18T23:03:59Z
type: epic
priority: 2
assignee: Max Omdal
tags: [versions, state, history]
---
# Versioning evolution: useful checkpoints, efficient state history, and reliable recovery

Ship the version-history foundation in PRs #149–#153, use it on real artifacts, and improve it from observed needs. Current boundaries are changed content saves, not completed requests; state-only writes are not independent undo points. Full state snapshots can outgrow source history. Reviews also identify concurrency, history-navigation, responsive-layout and query-scaling gaps. Preserve the existing history while evolving these seams.

## Design

Keep monotonic artifact versions, immutable referenced content/assets, atomic code-plus-state restore, owner scoping, isolated previews and restore-as-a-new-version. Checkpoints are a presentation/operation boundary above individual saves. State snapshot encoding belongs below the store interface and must remain lossless and compatible with current state_json history. Keeping internal saves while grouping their UI does not reduce storage. No automatic pruning or app-specific schema migration is authorized by this epic. First use a release incorporating #152 transcript-preservation commits de84400 and 2dd715e. Distinguish a completed request from a validated/correct artifact.

Known residual findings and evidence: https://github.com/momja/Exhibit/pull/149#issuecomment-6253743050, https://github.com/momja/Exhibit/pull/150#issuecomment-6253871261, https://github.com/momja/Exhibit/pull/151#issuecomment-6254001490, https://github.com/momja/Exhibit/pull/152#issuecomment-6254304318, https://github.com/momja/Exhibit/pull/153#issuecomment-6254508383. Child tickets carry concrete reproductions. Prioritize recovery correctness, measure representative workloads, then tune checkpoints and encoding.

## Acceptance Criteria

Existing versions, previews, restores and undo-of-restore survive every migration unchanged. Child work adds explicit request checkpoints including state-only operations, lossless bounded-replay state storage, attributable storage/performance measurements, concurrency-safe continuation and usable history on narrow screens. Decisions about checkpoint frequency and storage policy are backed by real usage and reproducible benchmarks; no history is silently discarded.

