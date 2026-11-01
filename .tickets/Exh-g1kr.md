---
id: Exh-g1kr
status: open
deps: []
links: [av-f3cp, av-g8oa]
created: 2026-11-01T23:24:44Z
type: chore
priority: 2
assignee: Max Omdal
tags: [ci, testing, e2e]
---
# Run browser integration CI after merge instead of gating MRs

Browser integration CI is too slow to reasonably run and support for every MR. Run it after merge to main, with visible failures, rather than making it an approval or merge requirement. This policy supersedes the per-PR browser gate proposed in [av-g8oa](av-g8oa.md) and [av-f3cp](av-f3cp.md).

## Design

The browser workflow is absent from current main, but feature/av-f3cp/playwright-integration contains .github/workflows/browser-tests.yml with the Browser regression job triggered on pull_request, push to main, and workflow_dispatch. It combines Go/gallery Node tests, chromedp component checks, and Playwright journeys.

- Run browser integration coverage on push to main (the merged result) and retain workflow_dispatch for explicit investigation. Remove automatic browser execution on pull_request or merge_group.
- Inspect live branch protection/rulesets and checked-in configuration; remove any Browser regression required-check entry without weakening unrelated merge requirements. Do not leave a missing required status that blocks merges.
- Preserve fast non-browser PR checks. Separate them from the slow browser job where necessary; include both Playwright and Chromium-dependent Go integration tests in post-merge coverage.
- Keep failing post-merge runs visibly failed and retain existing reports, traces, screenshots, and server logs. Do not use continue-on-error to make failures green.
- Update the earlier CI story, epic, and relevant test documentation to describe the new policy. Keep local browser commands opt-in.

## Acceptance Criteria

1. A merge to main triggers browser integration CI against the resulting main commit; manual dispatch remains available.
2. Opening or updating an MR, including merge-queue checks if configured, does not automatically run the slow browser suite or wait for its status to approve/merge.
3. Repository protection/rulesets do not require the browser check, and no stale required browser status leaves an MR blocked. Existing unrelated requirements, including Migration versions, remain intact.
4. Fast non-browser PR validation is preserved independently of browser installation/execution.
5. A failing post-merge browser run remains visibly failed with useful diagnostic artifacts; passing runs execute the browser tests rather than silently skipping them.
6. CI documentation and the earlier browser testing tickets consistently describe post-merge, non-gating browser coverage.

