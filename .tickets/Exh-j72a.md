---
id: Exh-j72a
status: open
deps: []
links: []
created: 2026-10-18T23:03:59Z
type: task
priority: 2
assignee: Max Omdal
parent: Exh-c2hu
tags: [versions, state, history]
---
# Bound history lookup, transcript I/O, and cleanup costs

Review measurements found unindexed blob reference lookups scanning all version/asset history (#149), native session files read fully before enforcing 64 MiB (#151), and summary queries traversing large SQLite overflow payloads because version_seq follows session_file (#151). These costs grow with retained history.

## Design

Add migration-safe indexes for version body/widget blob IDs and historical asset blob IDs; verify query plans. Read native session files with a hard allocation/read bound before accepting them. Keep transcript summary fields ahead of/outside large payloads using a new migration if the original has shipped. Preserve current native records and reference-safe deletion. Measure lock duration and peak allocation; do not renumber or rewrite an already deployed migration.

## Acceptance Criteria

Benchmark representative large histories and 32–256 MiB conversation inputs. Reference checks use indexes, large rejected files do not allocate/read the entire file, and listing summaries avoids payload reads. Current databases upgrade with conversations/versions intact; artifact/account cleanup remains complete and does not hold the sole SQLite connection for avoidable table scans.

