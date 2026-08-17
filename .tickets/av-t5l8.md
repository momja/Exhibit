---
id: av-t5l8
status: open
deps: []
links: [av-8gyd]
created: 2026-08-17T06:37:12Z
type: bug
priority: 0
assignee: Max Omdal
tags: [widget, store, regression]
---
# Widget saves 500 on main: widget_blob_id dropped from the updatable-column allowlist

`main` is currently broken. 28 tests fail in `internal/api`, and the entire widget surface 500s.

Commit `9209abc` ("fix: apply CodeRabbit auto-fixes", authored by the CodeRabbit bot and merged to main) removed `widget_blob_id` from `updatableArtifactColumns` in `internal/store/sqlite.go:326` without updating `putWidget`, which sets that column through `UpdateArtifact`. Every widget save therefore fails with `"widget_blob_id" is not an updatable column`.

Reproduce: `go test ./internal/api/` on `main`.

Deployed instances built from `main` since that commit cannot save a widget — including the agent's `set_widget` tool and the edit page's "Generate widget" button.

## Design

**Excluding the column from the PATCH allowlist is correct and should stand.** A PATCH able to set `widget_blob_id` could repoint a card's tile at any blob, which is exactly the kind of write the allowlist exists to prevent. The bug is that the caller was not updated to match.

The fix is to give the attach its own store method — `AttachWidget` — rather than routing it through the general `UpdateArtifact` PATCH path. That is symmetric with `DeleteWidget` and keeps the allowlist honest.

**This fix already exists**, written as a prerequisite while implementing [[av-8gyd]] (the widget half of that ticket's acceptance criteria could not be exercised at all with the surface 500ing). It currently sits on `feature/av-8gyd/blob-deletion-queue`.

The open question is delivery: leave it bundled with av-8gyd, or cherry-pick it onto its own branch so `main` can be fixed without waiting on a feature review. Bundling an unrelated P0 regression fix inside a feature branch delays the fix and muddies the review of both.

## Acceptance Criteria

- `go test ./internal/api/` passes on `main`.
- Saving a widget through the API, the agent's `set_widget` tool, and the edit page's Generate button all succeed.
- `widget_blob_id` remains absent from `updatableArtifactColumns`; the attach goes through a dedicated store method.
- A test covers the attach path directly, so a future allowlist edit cannot silently break it again.

