---
id: Exh-5z7b
status: in_progress
deps: []
links: []
created: 2026-10-25T00:33:30Z
type: bug
priority: 1
assignee: Max Omdal
tags: [agent, ui]
---
# Show provider authentication failures in agent chat

Provider failures are delivered in assistant message_end events but agent.js ignores them. The deployed agent silently shows no reply when OpenRouter returns 401 User not found or OpenCode reports an expired key, so users suspect that saving keys or the backend is broken.

## Acceptance Criteria

Show one useful provider error in chat when a final assistant message fails; keep cancellation and retries correct; add focused browser-module tests; verify with an actual provider failure.

