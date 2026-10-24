---
id: av-g8oa
status: closed
deps: [av-e1sr]
links: []
created: 2026-07-07T05:44:36Z
type: feature
priority: 1
assignee: Max Omdal
parent: av-f3cp
---
# e2e: first GitHub Actions workflow and required browser checks

Second step in [av-f3cp](av-f3cp.md), immediately after the [harness](av-e1sr.md). This is the minimal CI slice brought forward before wider UI coverage. The repository has no Actions workflow today. Use GitHub-hosted Ubuntu; no self-hosted runner or persistent VM environment is required.

## Design

- Create a workflow for every PR targeting main, pushes to main, and manual dispatch. Give its test check a stable name. Keep the workflow trigger unconditional so path filters cannot leave a required check pending. Automatic runs with changes only under `.tickets/` report success without the test workload; mixed changes and manual dispatch run the full suite. Use ordinary PR execution without repository secrets.
- Set up Go from `go.mod` and a compatible, explicit Node LTS version; install `e2e/` with `npm ci`. Run `make build` to generate embedded assets before Go tests or app startup. Run `go test ./...` (which already invokes the gallery's Node suite) and the Playwright suite. An initial single hosted job can share the build/browser installation; no artifact-distribution pipeline is needed.
- Install the locked Playwright Chromium and Linux dependencies with `npx playwright install --with-deps chromium` in `e2e/`, before executing browser-dependent tests. Do not cache browser binaries initially: [Playwright's CI guidance](https://playwright.dev/docs/ci#caching-browsers) advises against it. Ordinary dependency caches are optional.
- Existing chromedp integration tests, including the Stimulus action-menu tests from [Exh-16ds](Exh-16ds.md), currently skip without a browser. Set `CHROMIUM_TEST_BINARY` explicitly to the executable from this Playwright installation, check that it exists and is executable, and verify the required component cases pass rather than skip. Keep their current optional local behavior. Optional S3/Pi integration suites are outside this first gate.
- Require the Playwright command to execute tests and fail on missing browsers, failures, empty discovery, or focused tests (`forbidOnly` in CI). Start with one worker, no automatic retries, `trace: 'retain-on-failure'`, and screenshots on failure. This retains the original failing attempt without relying on a retry to capture it. See [trace options](https://playwright.dev/docs/trace-viewer#recording-a-trace).
- Give app startup and the suite bounded timeouts, with the job limit comfortably longer so reports can finish. Upload `playwright-report/`, `test-results/` (including trace ZIPs/screenshots), and captured server logs on failure, including failures without a browser trace such as startup errors. Use failure-safe artifact steps and a documented retention period, initially 14 days.
- Document the stable check and the repository setting needed to require it before merge. A workflow alone does not enforce branch protection. The user approved enabling this required check on 2026-10-23; apply it after the check reports green. Do not silently enable unrelated rules. Main currently has no classic protection and its listed ruleset is disabled.

## Acceptance Criteria

1. A clean GitHub-hosted Ubuntu runner builds the app, runs Go/gallery Node tests and required existing Chromium component cases, and runs the flagship Playwright test against its own fresh two-origin app. No VM runner, deployment, external secrets, live LLM, or S3 service is needed.
2. An intentionally failing browser assertion produces a failed check and downloadable trace, screenshot, report, and useful app logs. A startup failure produces a failed check with logs. Missing browser or zero discovered tests cannot produce a green check. Demonstrate these in a temporary branch change and restore the passing test before handoff.
3. No browser binary cache, coverage guard, matrix, sharding, or retries is needed to accept this story. The later modal tests join the same suite automatically.
4. The check reports for all configured PR/push events, including ticket-only changes. Only automatic runs whose entire diff is under `.tickets/` skip tests; any other changed path runs the suite. Manual dispatch always runs tests. Record the agreed required-check rollout and verify merge enforcement once enabled; until enabled, report explicitly that merge enforcement is pending rather than calling it enforced.
5. Local `make test` and ordinary agent workflows do not acquire a Playwright requirement. The explicit frontend/debugging command remains available.

## Deferred work

Coverage mapping and maintenance guidance moved to [Exh-gtz1](Exh-gtz1.md). Browser caching is a measured optimization, not an acceptance criterion. A blocking changed-path guard needs a separate usefulness decision after real coverage exists.

## Implementation — 2026-10-23

Implemented the first hosted Ubuntu workflow with Node 24, Go from go.mod, locked Chromium, existing Go/Node/component checks, six Playwright journeys, and 14-day failure evidence. Selects the installed headless shell through Playwright public APIs and rejects skipped required component results. actionlint passed. Hosted failure/green-run validation and required-check activation are PR rollout steps; user approved merge enforcement on 2026-10-23.

## Hosted failure proof — 2026-10-23

[Run 39906606070](https://github.com/momja/Exhibit/actions/runs/39906606070) passed all Go/Node/required Chromium checks and all six real Playwright journeys. Only the temporary intentional assertion failed. Downloaded and inspected its artifact: trace ZIP, failure screenshot, HTML report, server log, and Go JSON results are present. The probe is removed from the final suite. The approved required-check rule will be activated after the restored run is green; rollout evidence is recorded on [PR #198](https://github.com/momja/Exhibit/pull/198).

## Ticket-only exception — 2026-10-24

Automatic PR/push runs compare the complete change, including both sides of renames. If every changed path is under `.tickets/`, the same required check reports success without installing tools or executing tests. Mixed changes run the suite. Manual dispatch and unavailable comparison bases do not skip tests; comparison errors fail the check. Workflow triggers remain unfiltered.

Validation: actionlint passed, and the exact workflow classifier passed 22 cases in a temporary Git repository, including ticket additions/edits/deletions, mixed and multi-commit changes, renames across the directory boundary, unusual filenames, manual runs, invalid bases, and a PR merge after main advanced.
