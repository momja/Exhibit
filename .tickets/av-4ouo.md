---
id: av-4ouo
status: closed
deps: []
links: []
created: 2026-07-05T05:58:25Z
type: chore
priority: 2
assignee: Max Omdal
---
# Add pre-push documentation check hook

Configure a PreToolUse agent hook on git push that checks whether docs/README/AGENTS.md are consistent with the changes being pushed, fixes them if stale, commits the fix, then allows the push to proceed.

