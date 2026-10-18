---
id: Exh-sys1
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
# Measure version storage and deduplicate repeated code and state

Current blob accounting includes retained body/widget/assets, but SQLite state snapshots and conversation records need explicit attribution. Source/widget saves allocate fresh blob IDs; historical identical text can be stored repeatedly. Retention decisions currently lack measurements.

## Design

Report live versus retained code/widget/assets/state/conversation bytes with logical and physical sizes distinguished. Avoid double-counting shared owner-scoped blobs. Reuse identical snapshots and body/widget content without exposing another owner's data. Establish representative benchmarks and lightweight operational measurements for save/restore latency, state size, history depth and database growth. Keep deletion/refcount/cleanup semantics. Start with accounting and evidence; leave automatic pruning and quota policy to an explicit later decision.

## Acceptance Criteria

Representative fixtures explain where bytes grow and prove shared content is counted once in the defined accounting. A repeated A→B→A content save can share A's bytes. Artifact/account deletion reclaims every retained dependency safely. Document observed costs and any accounting limitations; no content is deleted to meet an inferred retention policy.

