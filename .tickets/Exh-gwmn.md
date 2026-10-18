---
id: Exh-gwmn
status: open
deps: []
links: []
created: 2026-10-18T23:03:59Z
type: bug
priority: 2
assignee: Max Omdal
parent: Exh-c2hu
tags: [versions, state, history]
---
# Coordinate restores and resumed conversations with active writers

Remaining review findings: #149 frame write preconditions trust an optional iframe-provided version; #151 transcript version is sampled later than the settled conversation; #152 a rollback can race an agent active in another tab, and a failed rollback can DELETE an attached live session. These can cause surprising state or continuation behavior even though artifact snapshots remain recoverable.

## Design

Bind browser frame writes to trusted load/generation context while preserving a separate explicit API/inspector write path. Coordinate continuation/restore with the target session prompt lifecycle and reject or settle active writes. Distinguish session created versus attached, cleaning up only a process owned by the failed attempt. Capture the conversation's artifact-version provenance with its settled snapshot instead of reading the head at later DB-write time. Do not hold global registry locks through DB/process work.

## Acceptance Criteria

Deterministic tests cover missing/forged generation, legacy frame writes after restore, manual API writes, an in-flight agent from another tab during rollback, missing historical blobs and failed restore after attachment. Existing live sessions survive a failed attaching client. A delayed transcript write cannot acquire an unrelated newer manual-edit version. Preserve append-only recovery and owner scoping.

