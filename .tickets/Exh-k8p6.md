---
id: Exh-k8p6
status: closed
deps: []
links: [av-1rvm, Exh-6fz9, Exh-vfks]
created: 2026-10-19T04:37:07Z
type: feature
priority: 2
assignee: Max Omdal
---
# Export artifact localStorage as JSON from the edit page

Add Export JSON to the Stored state panel. Download the current persisted localStorage map as a .json file, preserving string values and arbitrary keys. Do not export unsaved inspector edits. Handle loading and errors using existing page credentials.

## Acceptance Criteria

An owner can download saved artifact state as a JSON object, including empty state. Failed reads report an error and do not produce a download. Unsaved edits and saved state are unchanged.


## Notes

**2026-10-19T04:42:02Z**

Implemented Export JSON in Stored state using a native ES module and existing authenticated state GET. Verified lossless string export, empty state, failed reads, retries, and duplicate clicks with six tests. make assets, go build ./cmd/server, and go test ./... pass. Browser download exactly matched persisted state; desktop and mobile screenshots saved in docs/screenshots/Exh-k8p6/.
