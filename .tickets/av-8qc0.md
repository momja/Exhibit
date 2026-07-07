---
id: av-8qc0
status: open
deps: [av-e1sr]
links: []
created: 2026-07-07T05:44:23Z
type: feature
priority: 2
assignee: Max Omdal
parent: av-f3cp
---
# e2e: security boundary tests — allowlist enforcement + sandbox containment

fetcher.html fixture (button fetches a configurable origin, reports outcome into its DOM) and escape.html fixture (attempts window.parent DOM, cookies, app-origin requests; reports each outcome). The blocked-origin target is a local http server started by the test on a third port, so 'blocked' is asserted at the would-be destination, not inferred from the page. Cover: empty allowlist -> fetch blocked + nothing arrives; approve origin (PATCH allowlist) -> fetch succeeds after re-render; pre-approved origin -> no prompt; every sandbox escape attempt fails. See docs/proposals/e2e_testing.md §4.3, §5 items 2-3.

## Acceptance Criteria

GIVEN fetcher.html with an empty allowlist WHEN it fetches the test-owned origin THEN the request never reaches the destination server and the page reports failure. GIVEN the origin is approved via PATCH THEN the fetch succeeds after re-render. GIVEN escape.html WHEN it attempts parent-DOM/cookie/app-origin access THEN every attempt is reported failed.

