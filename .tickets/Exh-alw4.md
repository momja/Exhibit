---
id: Exh-alw4
status: in_progress
deps: []
links: []
created: 2026-10-23T23:26:34Z
type: bug
priority: 2
assignee: Max Omdal
parent: Exh-g2vv
tags: [experimental, relay]
---
# [Experimental] Resume artifact reads after relay throttling

A connected ticket visualizer loads one GitHub blob per ticket with six workers. Its PAT works, but loads over the relay burst/rate budget fail with request limit reached. Preserve relay limits and return an accurate Retry-After for local throttling; the trusted host should wait and retry bounded GET/HEAD requests. Do not replay writes, provider responses, daily quota failures or permission failures. Abort, navigation and policy changes must stop retries.

## Acceptance Criteria

A browser load can finish after local relay throttling. Local 429 responses carry accurate retry guidance. Retried reads recheck current permissions; writes and unrelated failures are not replayed. Retries respect cancellation and a bounded deadline. Add focused HTTP/browser regressions and update relay documentation.

