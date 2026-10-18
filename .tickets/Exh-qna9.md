---
id: Exh-qna9
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
# Checkpoint user operations and agent requests, including state-only changes

A single user request may save intermediate source/widget content several times; ordinary saved-state changes and inspector clear currently create no content version. Define meaningful recovery points without losing fine-grained saves.

## Design

Before the first write of an operation, preserve the outgoing code/widget/assets/state atomically. Group writes by a durable request/operation identity, not a whole agent session or an LLM-selected label. Finalize when the request and its tools settle; represent failure/cancellation as partial work. Manual saves, refetches and restores have explicit boundaries. Keep ordinary usage live; capture it before the next modifying operation. Concurrent independent operations must not be silently combined. API/storage design must allow a checkpoint with unchanged code.

## Acceptance Criteria

One request with three saves yields one primary history entry with expandable underlying saves; three requests in one session remain distinct. Read-only requests create none. State-only edits, clear and cancelled/failed requests have tested recovery behavior. Restore remains a new version and undo restores the exact outgoing state. Labels/completion do not claim validation.

