---
id: Exh-k8p6
status: in_progress
deps: []
links: [av-1rvm]
created: 2026-10-19T04:37:07Z
type: feature
priority: 2
assignee: Max Omdal
---
# Export artifact localStorage as JSON from the edit page

Add Export JSON to the Stored state panel. Download the current persisted localStorage map as a .json file, preserving string values and arbitrary keys. Do not export unsaved inspector edits. Handle loading and errors using existing page credentials.

## Acceptance Criteria

An owner can download saved artifact state as a JSON object, including empty state. Failed reads report an error and do not produce a download. Unsaved edits and saved state are unchanged.

