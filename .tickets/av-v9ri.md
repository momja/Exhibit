---
id: av-v9ri
status: closed
deps: []
links: []
created: 2026-09-27T20:02:25Z
type: bug
priority: 1
assignee: Max Omdal
tags: [testing, api]
---
# internal/api tests fail to build: stale renderEditPage call

`go test ./...` fails on main: internal/api's test package no longer compiles.

    internal/api/gallery_test.go:568:107: not enough arguments in call to renderEditPage
        have (*store.Artifact, nil, string, string, pageCredentials, renderURLs, bool, string)
        want (*store.Artifact, []store.OriginDecision, []*store.Tag, string, string, pageCredentials, renderURLs, bool, string)

66498b3 (Move tag editing from the gallery grid to the edit page) added the `library []*store.Tag` parameter to renderEditPage and updated every call site but TestEditPageNeverOffersTheRenderOrigin, which still passes the old argument list. Every other Go package builds and tests fine, so the whole `go test ./...` run fails on that package before any api test executes.

Fix: pass nil for the tag library in that one call, matching the other origin-only tests in the file. The same one-line repair already sits as a drive-by on the open av-3cdp branch (PR #133); landing it separately unblocks main now instead of waiting on that PR.

## Acceptance Criteria

go test ./... passes on the branch; no other package regresses.

