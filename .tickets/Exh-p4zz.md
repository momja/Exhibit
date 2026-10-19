---
id: Exh-p4zz
status: in_progress
deps: []
links: []
created: 2026-10-19T19:47:55Z
type: bug
priority: 2
assignee: Max Omdal
tags: [security, agent, dos]
---
# Reserve agent process capacity atomically before spawning

The ten-session owner limit is checked before startup and reconciled after spawning, allowing concurrent Creates to spawn excess subprocesses transiently. Reserve startup capacity atomically, release it on failure and process exit, and add an instance-wide process ceiling. Preserve idle eviction and readable 429 responses. Test concurrent admission and verify refused requests never spawn. Split from av-4bzn.

