---
id: av-e1sr
status: closed
deps: []
links: []
created: 2026-07-07T05:44:12Z
type: feature
priority: 1
assignee: Max Omdal
parent: av-f3cp
---
# e2e: isolated harness and state round-trip flagship

First implementation step in [av-f3cp](av-f3cp.md), authorized on 2026-10-23. Establish a small Playwright workspace and one test proving the storage bridge works through the built application.

## Design

- Add a top-level `e2e/` workspace with a pinned `@playwright/test`, lockfile, Chromium project, and configuration. Keep it outside `web/`, whose package manifests are automatically installed by `scripts/build-assets.sh`.
- A managed test-server launcher starts `bin/server` with a newly created scratch `DATA_DIR`, filesystem blobs, and explicit loopback `ADDR`, `RENDER_ADDR`, `APP_ORIGIN`, and `RENDER_ORIGIN`. Default ports are 18080/18081: distinct ports are distinct browser origins. Keep `SINGLE_LISTENER` off. Wait for both surfaces to be ready and fail on occupied ports rather than attach to an existing application (`reuseExistingServer: false`). Stop the child process and remove scratch data on completion/failure; expose startup logs on failure.
- Supply a controlled test environment so inherited OIDC, agent, S3, public-instance, or deployment configuration cannot redirect this run into external services. No production data or credentials.
- Bootstrap a fixture owner using the existing supported local-auth path. Log in through the real local-login endpoint before seeding and retain its app session for owner contexts. Fresh-server startup now creates/enables local login; bearer headers alone do not authenticate page navigations. Ensure the fixture session owns the artifacts seeded with the service bearer token (owner 1 on the fresh store). Never place a global Authorization header on all browser requests.
- Add API helpers for ingest and cleanup through the ordinary endpoints. Use unique fixture titles/IDs per test, fresh contexts, and one worker initially. No direct SQLite fixture edits and no test-only production routes. Account provisioning may use the supported server CLI before startup if needed.
- `fixtures/counter.html` reads its counter from `localStorage` synchronously during startup, displays it, and increments/persists on click. Open it through the authenticated app detail page and its real sandboxed render iframe; do not bypass the host bridge by opening the render URL as a top-level page.
- Add explicit `make e2e` and `make e2e-ui` targets and setup instructions. Document the clean-checkout sequence (`make build`, workspace install, Chromium install, test command), tool versions, data/port isolation, and failure investigation. Missing setup must fail with useful output. Ordinary `make test` remains unchanged.
- Use role/label locators and Playwright frame locators, observable API/DOM conditions, bounded timeouts, and failure traces. Do not mock persistence/rendering or use arbitrary sleeps to hide races.

## Acceptance Criteria

1. From a clean checkout, documented setup followed by `make e2e` builds on the existing `make build` prerequisite and runs the real app and Chromium without a deployment or third-party service.
2. GIVEN an API-seeded counter artifact and a logged-in owner, WHEN it opens in the app iframe and is clicked twice, THEN its display becomes 2 and an authenticated state read eventually confirms the persisted counter is 2. Wait for the write before navigating.
3. WHEN that page reloads, THEN the artifact's synchronous startup read displays 2. WHEN a fresh browser context logs in as the same owner and opens it, THEN its startup display is also 2, with no copied browser storage or test-side state injection. The DOM value and server state must agree.
4. The test uses two actual origins, the current iframe sandbox (no `allow-same-origin`), and no-store render responses. A broken shim → postMessage → host state write → render-state rehydration path makes it fail.
5. Repeated runs use fresh data and contexts. Startup failure, occupied ports, missing browser, or an empty/misconfigured suite fails instead of reporting success or reusing the user's server. Normal success/failure leaves no app process running.
6. Frontend contributors can run this explicitly; unrelated changes do not gain a mandatory local browser step. Browser dependencies are absent from production asset builds.

## References

[Security and storage boundary](../docs/security.md), [current startup/auth](../cmd/server/main.go), [API](../docs/api.md). The missing July proposal is superseded by this design.

## Implementation — 2026-10-23

Implemented the isolated built-server launcher, API fixtures/real local login, and flagship state round-trip through reload and a fresh owner context. The state journey passes locally. Verified occupied-port rejection, missing-build/browser and empty-suite failures, deployment-env isolation, and SIGTERM process/data cleanup. Explicit Make targets and setup/debugging guide added; ordinary make test unchanged.
