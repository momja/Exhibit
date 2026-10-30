---
id: Exh-t08t
status: closed
deps: []
links: []
created: 2026-10-30T03:01:10Z
type: chore
priority: 2
assignee: Max Omdal
---
# Remove unreachable Go code with deadcode

Run golang.org/x/tools/cmd/deadcode across all Go entry points, including tests. Review reported functions and remove obsolete code while retaining required interfaces and supported behavior.

## Acceptance Criteria

Reviewed unused functions removed; deadcode findings documented; relevant build and tests pass.


## Notes

**2026-10-30T03:06:24Z**

Used golang.org/x/tools/cmd/deadcode v0.52.0, installed separately from the project module (project Go directive and dependencies unchanged). Initial test-inclusive analysis found Router.hasServiceToken and blobtest.Configured with no callers. Production analysis also exposed obsolete putBlob/galleryURL wrappers, keyedLimiter.penalise, and color.ContrastText used only by their own tests. Removed all six, the unused contrast constants, and stale comments. Existing storage/gallery tests now call current implementations; limiter budget/refill/memory tests now exercise reserve, the atomic debit used by login. Retained genuine test support: S256Challenge, keyedLimiter.size, NewFetcherForTests, and scanner.Scan. Analysis excludes npm dependency Go files by targeting ./cmd/... ./internal/.... Post-cleanup deadcode -test -tags=screenshot ./cmd/... ./internal/... reports no findings. Validation in progress.

**2026-10-30T03:13:26Z**

Validation so far: make assets, go build ./cmd/..., screenshot-tag API compilation, color/blob tests, and focused API tests (84.776s) passed. Baseline go test ./... finished with one pre-existing browser timeout: TestGallerySpacesBrowser/desktop, galleryspaces_browser_test.go:83, context deadline exceeded. The same browser test passed in the focused post-change run. Full post-change suite still running.

**2026-10-30T03:14:23Z**

Final validation: go test ./... PASSED (API 514.965s, store 277.850s), including the gallery browser test that timed out on the baseline. go build ./cmd/..., focused API tests, screenshot-tag API compilation, and git diff --check passed. deadcode -test -tags=screenshot ./cmd/... ./internal/... exited successfully with zero findings. Cleanup removes six functions and unused contrast constants; existing tests cover current implementations.
