---
id: av-f3cp
status: open
deps: []
links: []
created: 2026-07-06T22:06:39Z
type: epic
priority: 1
assignee: Max Omdal
---
# Browser regression testing with Playwright

Add real-browser regression coverage to CI for the isolated artifact runtime and the gallery's server-rendered UI. The immediate reason to bring this forward is the [Stimulus foundation](Exh-16ds.md) ([PR #187](https://github.com/momja/Exhibit/pull/187)) and the next shared modal component.

## Planning state — 2026-10-20

This is a ticket refresh for discussion. All implementation stories remain open; do not start implementation until the user has discussed this plan. No application code, browser installation, workflow, VM runner, or repository rules are being changed in this pass.

The July proposal `docs/proposals/e2e_testing.md` is absent from main and the locally available `origin/feature/av-f3cp/e2e-testing` tree. These tickets now carry the actionable design and acceptance criteria themselves. Current architecture references are [security](../docs/security.md), [UI components](../docs/ui-components.md), and [technical stack](../docs/technical_stack.md), rather than missing proposal sections.

## Delivery sequence

1. [av-e1sr: isolated harness and state round-trip](av-e1sr.md). Run the built application with fresh storage, two real origins, real login, and API-seeded fixtures. Start with Chromium and one worker.
2. [av-g8oa: minimal hosted CI gate](av-g8oa.md). Establish the repository's first GitHub Actions workflow on GitHub-hosted Ubuntu. Install the browser, execute existing Go/Node checks and required browser suites, and retain failure evidence. This moves ahead of wider test coverage and maintenance automation.
3. [av-f05n: modal and htmx integration flow](av-f05n.md). Cover menu → modal → save → server-rendered htmx refresh → usable replacement component, including keyboard and focus. Coordinate with the upcoming modal implementation; the proposed first consumer is gallery Duplicate, subject to discussion.

Then expand coverage with [av-8qc0: runtime security boundaries](av-8qc0.md), [Exh-jtv1: ingest/sharing/search/tags](Exh-jtv1.md), and [Exh-gtz1: coverage inventory and maintenance guidance](Exh-gtz1.md). These remain part of the epic, but do not delay the initial CI milestone. Security can proceed after the harness and CI without waiting for modal delivery.

## Scope and operating model

- Playwright is a CI regression suite, with an explicit local command for frontend work that affects covered behavior or for investigating CI failures. It is not added to ordinary `make test`, app startup, asset builds, or a blanket agent checklist.
- Keep `e2e/` outside `web/`: the asset builder installs every immediate `web/*/package.json` workspace. Test dependencies must not become production assets.
- Use fresh local data, fixture credentials, and local HTTP targets. No deployed environment, production secrets, external identity provider, S3 service, or live LLM is needed for these flows.
- Preserve existing Go and Node coverage. Existing chromedp component tests exercise Stimulus lifecycle and focus; Playwright adds full application journeys. Do not port every existing component assertion into a second suite.
- Start without a browser cache, browser/OS matrix, sharding, visual baselines, or a blocking changed-path coverage guard. Revisit those only when measured runtime or demonstrated coverage gaps justify them.

## Feasibility and discussion points

- Hosted CI is the recommended environment. The public repository currently has no Actions workflows. Go, Node-built embedded assets, filesystem blobs, SQLite, and two loopback listeners fit a standard hosted Ubuntu runner; a runner on this VM is not needed for the proposed scope. Runtime and reliability still need to be measured in implementation.
- The built server now enables login on a fresh database. The harness must establish a fixture owner session and ensure bearer-token seeding and UI login resolve to the same owner; the old unauthenticated-page assumption is stale.
- Agree which feature first adopts the shared native-dialog controller and identify its implementation ticket. No such modal story was found in the current ticket set. Existing Duplicate is a concrete candidate, but its current dialog is feature-owned.
- Agree the merge-gate policy. A failing workflow check and a repository rule requiring that check are separate changes. The classic branch-protection endpoint currently reports main unprotected, and the listed "Main protection" ruleset is disabled. Enabling required checks belongs to the CI rollout discussion; this pass changes no settings.
- Sharing UI exists, and anonymous shares now live until revoked/replaced; expiry was removed. The old API-only share journey and expiry criterion are superseded.

## Acceptance Criteria

- The first milestone is complete when the state round-trip and selected modal journey run as enforced CI tests with useful failure traces and the replacement component works after a real htmx response.
- The epic is complete when the initial milestone and the later security, broader UI, and coverage-maintenance stories are complete, with remaining gaps explicitly recorded.
- Local browser execution remains opt-in for relevant frontend work; CI must not silently skip its required browser coverage.

## Notes

**2026-07-07T05:44:59Z** — Historical plan referenced a proposal on the epic branch and grouped CI/currency and all UI journeys together. Superseded by the staged plan above on 2026-10-20.
