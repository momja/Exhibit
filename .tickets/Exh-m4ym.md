---
id: Exh-m4ym
status: closed
deps: [Exh-ky6e]
links: []
created: 2026-07-11T05:19:45Z
type: task
priority: 1
assignee: Max Omdal
parent: Exh-yvhp
---
# Pi sidecar session manager in Go (spawn pi --mode rpc, stream events)

internal/agent package: spawn pi --mode rpc --no-session per chat session with decrypted key in env; strict JSONL framing; command/response correlation; broadcast agent events to SSE subscribers; session lifecycle (create/prompt/abort/close, idle reaper).

