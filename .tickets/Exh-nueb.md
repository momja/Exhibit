---
id: Exh-nueb
status: open
deps: []
links: []
created: 2026-10-19T19:47:55Z
type: bug
priority: 2
assignee: Max Omdal
tags: [security, agent, dos]
---
# Bound transcript file reads before allocating their contents

readSessionFile uses os.ReadFile before enforcing maxSessionFileBytes (64 MiB). Persistence is bounded but memory allocation is not. Read with a byte limit plus an overflow sentinel, preserving complete-line handling and refusing oversized files without allocating their full contents. Cover boundary and unterminated oversized files. Split from av-4bzn.

