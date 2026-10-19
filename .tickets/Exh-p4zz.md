---
id: Exh-p4zz
status: closed
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


## Notes

**2026-10-19T21:42:38Z**

Atomically reserve process capacity before startup: ten processes per owner and 32 per instance, counting pending starts and stopping processes until Wait returns. Failed startup releases its reservation. Reclaim only the requesting owner’s oldest idle process and wait for it to be reaped before reusing capacity. Both ceilings return readable 429 responses. Concurrent owner/global admission, no-spawn refusal, startup failure, idle eviction, and process exit tests pass under the race detector; the full agent race suite also passes.

**2026-10-19T21:45:26Z**

Final validation passed: full go test -race ./internal/agent (97.434s), full go test ./internal/api (246.253s), and final targeted admission race tests (18.252s).
