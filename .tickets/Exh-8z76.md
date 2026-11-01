---
id: Exh-8z76
status: in_progress
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

