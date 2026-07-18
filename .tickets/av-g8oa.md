---
id: av-g8oa
status: open
deps: [av-e1sr]
links: []
created: 2026-07-07T05:44:36Z
type: feature
priority: 2
assignee: Max Omdal
parent: av-f3cp
---
# e2e: CI workflow + suite currency mechanisms

GitHub Actions workflow (free on public repos): make build, go test ./..., then Playwright job — actions/cache on ~/.cache/ms-playwright keyed on runner OS + Playwright version from e2e/package-lock.json; install-deps still runs on cache hit; trace on-first-retry; upload playwright-report + traces on failure. Plus the currency mechanisms: e2e/COVERAGE.md (claim -> spec -> ticket map seeded from proposal §1 table), CLAUDE.md update rule (change browser-enforced behavior => update specs+COVERAGE.md in-branch OR file 'e2e:' tk ticket with given/when/then criteria), and a CI path-filter guard that flags changes to internal/render//shim/CSP/gallery JS with no e2e/ change and no e2e ticket reference. See docs/proposals/e2e_testing.md §6, §8.

## Acceptance Criteria

GIVEN a PR WHEN CI runs THEN go tests and the Playwright suite both execute and failures upload traces. GIVEN a warm cache THEN the browser download step is skipped. GIVEN a PR touching internal/render/ with no e2e/ change and no e2e ticket reference THEN the guard step flags it. COVERAGE.md exists and maps every §1 claim to a spec or an explicit gap.


## Complexity

M
