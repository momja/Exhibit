---
id: Exh-8z76
status: closed
deps: []
links: []
created: 2026-11-01T17:32:40Z
type: bug
priority: 1
assignee: Max Omdal
tags: [security, auth, clerk]
---
# Clerk login accepts a triple-slash next and redirects off-origin

safeNext (internal/api/auth.go) accepts next values such as ///evil.test and ////evil.test: Go's url.Parse reads them as paths with an empty host. Local and OIDC logins are safe because http.Redirect path-cleans the value, but the Clerk login page passes next to clerk-controller.js, which calls window.location.replace(next) (and Clerk's forceRedirectUrl); the browser resolves ///evil.test against the app origin to https://evil.test/. A signed-in Clerk user following /auth/login?next=%2F%2F%2Fevil.test is sent off-origin from a genuine Exhibit link (phishing). Found by the Codex security review of PR #238; pre-existing on main.

## Acceptance Criteria

safeNext rejects any next starting with // (after the existing control-character/backslash checks); regression tests cover ///host, ////host, /%2F/host style inputs for local, OIDC and Clerk paths; the Clerk page never receives an off-origin next.


## Notes

**2026-11-01T17:41:23Z**

Reject every leading // sequence in safeNext after the control-character/backslash checks. Regression tests demonstrated the original failure for ///evil.test and ////evil.test at safeNext, Clerk login/signup HTML attributes, OIDC next cookies/callbacks, and local login. Keep /%2F/evil.test unchanged: browsers preserve its encoded slash as same-origin path data; controller tests cover this and both forceRedirectUrl modes. Audited local/error/throttled login renders, SSO/callback cookies, sessionGate, Clerk sign-in/sign-up/logout and handshake returns, editor return_to, gallery-return/edit.js, and unsaved-changes navigation. No additional exploitable return-value gap found.

**2026-11-01T18:02:29Z**

Validation: make assets passed. The focused Go regression command failed before the production fix and passed afterward (7.617s). node --test web/gallery/*clerk*.test.mjs passed 17/17. go build ./... and go vet ./... passed. The default go test ./... hit the 10-minute internal/api timeout at 600.178s; all other packages passed. The complete rerun go test -json -timeout=30m ./... passed with exit 0 (internal/api 775.166s), with 2323 passing test/subtest events and 16 existing skips; no added test filters or further production changes. git diff --check passed. Independent review found no blocking findings.
