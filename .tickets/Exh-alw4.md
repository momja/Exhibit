---
id: Exh-alw4
status: closed
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


## Notes

**2026-10-23T23:39:10Z**

Confirmed the user's PAT connection works after the artifact grant: GitHub reads succeeded, but the six-worker GitHub blob loader exceeded Exhibit's 60-request artifact burst with a 60/minute refill. The trusted host now retries only GET/HEAD requests rejected with the local relay_rate_limited code, honoring numeric Retry-After within a one-minute retry budget. The server computes the next available token across owner/artifact/origin buckets without spending partial budgets on refusal. Rates remain unchanged. Every retry rechecks current policy; writes, provider responses, quota errors and permission failures are not replayed. Abort/navigation cancels the waiting timer, and late background timers cannot outlive the retry deadline.

Validation passed: gallery asset build; all 388 gallery Node tests (NODE_OPTIONS=--experimental-vm-modules npm test); focused HTTP and Chromium relay tests; go test -race ./internal/api -run TestRelay -count=1 -timeout 3m (32s); go vet ./internal/api; diff checks. A review finding about late timer wakes was fixed with its own regression. Updated relay/API docs. Deployed to the existing demo and completed a live read-only import of momja/Exhibit@main: 351 tickets loaded, 49 open / 13 in progress / 289 closed, and zero pending relay requests. No credential, permission or artifact source changes were needed.
