# Browser regression tests

Playwright runs in CI against its own built Exhibit server and scratch data.
Run it locally when changing covered frontend behavior or investigating a CI
failure. It is deliberately separate from ordinary `make test`, asset builds,
and application startup. Node 24 and the Go version in `go.mod` are required.

## First run

From the repository root:

```sh
make build
npm ci --prefix e2e
cd e2e
npx --no-install playwright install --with-deps chromium
cd ..
make e2e
```

The install command may need administrator access for Linux system libraries.
Once they are installed, `npx --no-install playwright install chromium` is enough
to fetch a missing browser. This workspace pins the browser through its locked
Playwright version. Node/test dependencies never enter the production assets.

After changing server code or embedded frontend assets, run `make build` again
before testing. `make e2e` uses `bin/server`; it does not build implicitly.
`make e2e-ui` starts Playwright's interactive test UI with the same isolated app.

## Isolation and fixtures

The launcher reserves loopback ports 18080 (app) and 18081 (render) and refuses
occupied ports. It never attaches to a running developer server. Distinct ports
are real browser origins; this configuration does not claim cookie host
isolation. Each run gets fresh SQLite/filesystem data under the system temp
directory. The launcher stops the app and removes that directory on exit.
It supplies only its fixture environment, so deployment OIDC, S3, public-mode,
agent, and credential settings cannot leak into the run. The optional Pi binary
is unavailable to the app. No deployed services or repository secrets are used.

Fixtures log in through the normal local-login endpoint as the fresh server's
default admin, then create artifacts through a dedicated bearer-authenticated
API client. Browser requests use their normal session cookie, never a global
Authorization header. Unique artifacts are cleaned through the API. The state
test verifies the rendered iframe, persisted API state, reload, and a fresh
owner context without copied browser storage.

## Failures and CI

Open `e2e/playwright-report/index.html` using `npm run report --prefix e2e`.
Traces and screenshots live under `e2e/test-results/`; inspect a trace with:

```sh
cd e2e
npx --no-install playwright show-trace test-results/<failed-test>/trace.zip
```

Server startup/runtime output is in `e2e/artifacts/server.log`. Missing build,
browser, occupied port, zero discovered tests, and failing assertions fail the
run. CI forbids focused tests. One worker and zero retries keep the first
failure visible; `retain-on-failure` preserves its trace. App startup and test
execution have bounded timeouts.

GitHub Actions runs **Browser regression** on every PR targeting main, pushes
to main, and manual dispatch on GitHub-hosted Ubuntu 24.04. It installs Chromium,
builds assets/server, runs Go tests (including the gallery Node suite), verifies
the existing Stimulus Chromium tests passed rather than skipped, then runs
Playwright. Missing optional S3/Pi integrations remain outside this gate.
Failure artifacts retain reports, traces, screenshots, app logs, and Go JSON
results for 14 days. Browser binaries are not cached; only dependency downloads
are cached.

The repository's merge rule must require the **Browser regression** check from
GitHub Actions after its first green hosted run. A workflow file alone does
not enforce merges. Keep unrelated repository rules unchanged. Local Playwright
execution remains explicit; CI is the enforcement point.
