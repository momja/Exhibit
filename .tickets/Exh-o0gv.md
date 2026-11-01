---
id: Exh-o0gv
status: closed
deps: []
links: []
created: 2026-11-01T17:31:30Z
type: bug
priority: 1
assignee: Max Omdal
parent: Exh-g2vv
---
# Fix connector lifecycle, cache, and disabled-account review findings

Fix the five PR #197 review findings: durable cleanup for failed managed account completion; empty GET cache handling; request and response cache directives; disabled-account schedule authorization; bounded persistent cache retention.

## Acceptance Criteria

All five findings have regression coverage, appropriate checks pass, and operator documentation records cache and cleanup behavior.


## Notes

**2026-11-01T17:43:30Z**

Implemented the five findings with regression coverage: durable managed-account cleanup and restart retries; normal browser GET response reuse; request/response cache directives; disabled-account schedule guards before fetch and save; encrypted cache retention caps with metadata-only pruning. Focused checks pass. Final rebase and broad checks in progress. UI layout is unchanged, so existing connector screenshots remain current.

**2026-11-01T18:06:15Z**

Completed and rebased onto main 3c45711. All five findings have regression coverage. Failed managed saves and abandoned flows retain durable remote cleanup ownership; ordinary browser GET caching and no-cache/no-store behavior work; schedules verify enabled accounts before fetch and save; cache expiry/count/byte limits are enforced with covering-index metadata pruning. Reconnect identity and grants remain intact, and flow locks are released safely after failed setup. Full go test -json -timeout 20m ./... passed (23 tested packages, 2,713 passing cases); go vet ./..., make assets, all 439 gallery tests and focused connector lifecycle/lock race tests passed. Metadata pruning measured 34 ms at a 256 MiB cache versus 4.8 s before optimization. Documentation updated; no UI layout changed, so current screenshots remain applicable. Live provider validation and indeterminate provider-response reconciliation remain in Exh-3kvx.

**2026-11-01T21:44:58Z**

Reopened for current readiness blockers: rebase over the shared-modal migration and diagnose CI timeout in TestPersonalConnectionsBrowser/1280 while submitting the advanced reconnect form. Preserve modal lifecycle, connector access checks and navigation privacy during integration.

**2026-11-01 — Shared-modal integration**

Rebased onto main 8393af8d, retaining shared native dialog lifecycles, stale-preview checks, connector navigation policy checks and no-referrer navigation. Corrected the VM module test fixture and removed a duplicate component script include. All 444 gallery tests, vet, asset/server builds, six Playwright journeys and refreshed personal-connection screenshots pass. Reproduced the intermittent desktop reconnect submission timeout locally with CI Chromium 1248; diagnosis and full Go validation remain in progress.

**2026-11-01T22:26:10Z**

Fixed the remaining readiness blockers after rebase onto 8393af8d: reconnect browser clicks now wait for stable, unobstructed geometry after focus/scroll (45-second deadline unchanged); competing link reports are dropped before asynchronous policy checks during approval; browser fixtures use native dialogs, direct=1, and verify no-referrer before popup navigation. The pending-link regression was demonstrated red before the fix. Validation: all 445 gallery tests, final-source vet, 20 desktop plus 20 mobile reconnect journeys, all four previously failing modal browser cases, six application journeys, assets/server builds and updated screenshots pass. Full local baseline run is still completing; new CI will validate the final integrated commit. Live provider validation remains separate in Exh-3kvx.

**2026-11-01T22:38:49Z**

Rebased again onto main 5452fc54 after persisted library-agent grants landed. Kept both profile panels, credential protections and account-deletion cleanup. Preserved main migration 043; connector migrations now run from 044 through 053, with current documentation and upgrade-test boundaries updated. Upgrade tests from shipped versions 42 and 43 preserve existing saved data and agent grants. Expanded library-agent token denial coverage for connector/relay routes. Validation passed: assets build, all 452 gallery tests, final-source vet, affected API authorization/profile/deletion tests (42.251s), targeted migration/store deletion tests and Basic compatibility migration test. Previous e4e4521 passed both complete CI checks; the rebased commit will run CI again.
