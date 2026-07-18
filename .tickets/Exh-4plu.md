---
id: Exh-4plu
status: in_progress
deps: []
links: []
created: 2026-07-11T00:37:47Z
type: chore
priority: 3
assignee: Max Omdal
---
# Fix go.mod module path to match published repo

go.mod declares module github.com/artifact-viewer/artifact-viewer, a placeholder that doesn't match the actual repo at git@github.com:momja/Exhibit.git. Rename the module path (e.g. github.com/momja/Exhibit) and update every internal import path across the codebase to match.

## Acceptance Criteria

go.mod module line matches the real repo URL; all internal import paths updated repo-wide; project builds and tests pass.

