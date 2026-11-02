---
id: av-f3cp
status: in_progress
deps: []
links: [Exh-g1kr, av-g8oa]
created: 2026-07-06T22:06:39Z
type: epic
priority: 1
assignee: Max Omdal
---
# Browser regression testing with Playwright

Add real-browser regression coverage to CI for the isolated artifact runtime and the gallery's server-rendered UI. The immediate reason to bring this forward is the [Stimulus foundation](Exh-16ds.md) ([PR #187](https://github.com/momja/Exhibit/pull/187)) and the next shared modal component.

## Current delivery policy

The initial harness, hosted workflow, and Duplicate modal journey are implemented. Browser integration runs execute after merge to main and on manual dispatch; they do not run automatically for each MR or act as an approval/merge requirement. [Exh-g1kr](Exh-g1kr.md) supersedes the 2026-10-23 required-browser-check decision because that workflow is too slow for every MR. Later security, broader UI, and maintenance stories remain follow-ups.

The July proposal `docs/proposals/e2e_testing.md` is absent from main and the locally available `origin/feature/av-f3cp/e2e-testing` tree. These tickets now carry the actionable design and acceptance criteria themselves. Current architecture references are [security](../docs/security.md), [UI components](../docs/ui-components.md), and [technical stack](../docs/technical_stack.md), rather than missing proposal sections.

## Delivery sequence

1. [av-e1sr: isolated harness and state round-trip](av-e1sr.md). Run the built application with fresh storage, two real origins, real login, and API-seeded fixtures. Start with Chromium and one worker.
2. [av-g8oa: hosted browser regression CI](av-g8oa.md). On GitHub-hosted Ubuntu, install the browser, execute existing Go/Node/component checks and Playwright journeys after merge, and retain failure evidence. Preserve non-browser Go/Node validation on PRs and merge-queue events in a separate workflow. This moves ahead of wider test coverage and maintenance automation.
3. [av-f05n: modal and htmx integration flow](av-f05n.md). Cover menu → modal → save → server-rendered htmx refresh → usable replacement component, including keyboard and focus. Coordinate with the upcoming modal implementation; the approved first consumer is gallery Duplicate, with the shared-controller prerequisite tracked in [Exh-b5hv](Exh-b5hv.md).

Then expand coverage with [av-8qc0: runtime security boundaries](av-8qc0.md), [Exh-jtv1: ingest/sharing/search/tags](Exh-jtv1.md), and [Exh-gtz1: coverage inventory and maintenance guidance](Exh-gtz1.md). These remain part of the epic, but do not delay the initial CI milestone. Security can proceed after the harness and CI without waiting for modal delivery.

## Scope and operating model

- Playwright is a post-merge CI regression suite, with an explicit local command for frontend work that affects covered behavior or for investigating CI failures. It is not added to ordinary `make test`, app startup, asset builds, or a blanket agent checklist.
- Keep `e2e/` outside `web/`: the asset builder installs every immediate `web/*/package.json` workspace. Test dependencies must not become production assets.
- Use fresh local data, fixture credentials, and local HTTP targets. No deployed environment, production secrets, external identity provider, S3 service, or live LLM is needed for these flows.
- Preserve existing Go and Node coverage. Existing chromedp component tests exercise Stimulus lifecycle and focus; Playwright adds full application journeys. Do not port every existing component assertion into a second suite.
- Start without a browser cache, browser/OS matrix, sharding, visual baselines, or a blocking changed-path coverage guard. Revisit those only when measured runtime or demonstrated coverage gaps justify them.

## Feasibility and discussion points

- The implemented workflow uses a GitHub-hosted Ubuntu runner. Go, Node-built embedded assets, filesystem blobs, SQLite, and two loopback listeners fit that environment; a runner on this VM is not needed. Browser CI runs after merge because its runtime is too costly for every MR.
- The built server now enables login on a fresh database. The harness must establish a fixture owner session and ensure bearer-token seeding and UI login resolve to the same owner; the old unauthenticated-page assumption is stale.
- Approved 2026-10-23: Duplicate is the first shared native-dialog consumer; [Exh-b5hv](Exh-b5hv.md) implements the controller and migration before the Playwright flow.
- Browser failures remain visible with diagnostic artifacts, without becoming approval or merge requirements. Remove any stale browser required-check entries and preserve unrelated repository rules. This replaces the original 2026-10-23 merge-gate policy.
- Sharing UI exists, and anonymous shares now live until revoked/replaced; expiry was removed. The old API-only share journey and expiry criterion are superseded.

## Acceptance Criteria

- The first milestone is complete when the state round-trip and selected modal journey run in post-merge CI with useful failure traces and the replacement component works after a real htmx response. Browser CI does not run automatically on PRs or merge-queue events and is not required for approval or merge.
- The epic is complete when the initial milestone and the later security, broader UI, and coverage-maintenance stories are complete, with remaining gaps explicitly recorded.
- Local browser execution remains opt-in for relevant frontend work. Post-merge CI verifies browser coverage actually executes, except for the documented automatic ticket-only skip; manual dispatch always runs the suite. Non-browser PR validation remains separate.

## Notes

**2026-07-07T05:44:59Z** — Historical plan referenced a proposal on the epic branch and grouped CI/currency and all UI journeys together. Superseded by the staged plan above on 2026-10-20.
