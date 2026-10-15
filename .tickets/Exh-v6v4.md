---
id: Exh-v6v4
status: open
deps: []
links: []
created: 2026-07-12T03:30:17Z
type: task
priority: 2
assignee: Max Omdal
parent: Exh-i0ll
---
# Persist agent transcripts through the HTTP API (single-write-path fix)

Session.persistTranscript in internal/agent currently calls store.SaveTranscript directly — the one write bypassing the HTTP API. Add PUT /api/artifacts/:id/transcripts and route the agent manager through it. Prerequisite for extracting the agent to its own repository (Exh-k75k): once transcripts go over HTTP, the agent has zero store access. docs/agent.md §Extraction step 1

## Notes (av-y7td)

The row this writes now holds Pi's own session file (migration 032), which a later session can be started from. Authorize the PUT as the service rather than with the session's scoped credential: a session that can write its own history can plant instructions into a conversation the user later resumes. Today the write is the manager's and no model output reaches it.
