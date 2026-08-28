---
id: av-f5i5
status: open
deps: []
links: []
created: 2026-08-28T15:37:58Z
type: feature
priority: 2
assignee: Max Omdal
tags: [agent]
---
# Add edit tool for agent (pi-style targeted replacements)

Give the agent sidecar's exhibit.ts tool extension an edit tool modeled on pi's own edit tool, so multi-block targeted changes to an artifact don't require rewriting the whole body through update_artifact.

## Design

Modeled on pi's edit implementation: accepts {path, edits: [{oldText, newText}, ...]} (path is fixed to the session's bound artifact, per the existing single-artifact tool scoping in internal/agent/ext/exhibit.ts).

Matching: exact string match first; fall back to fuzzy matching that normalizes smart quotes -> ASCII, unicode dashes -> hyphens, and collapses extra/trailing whitespace, to tolerate copy-paste drift from model output.

Validation: reject if oldText is empty; reject if oldText is not unique in the current body (ambiguous match); reject if any two edits in the same call overlap or touch (agent must merge them into one edit instead).

Application: match all edits against the original body once, then apply replacements in reverse position order so earlier edits don't shift the offsets of later ones. When fuzzy matching kicked in for a given edit, rewrite only the touched lines from the normalized text and copy all untouched lines from the original byte-for-byte, preserving line endings (\r\n vs \n) and BOM.

Wiring: new tool alongside create_artifact/update_artifact/get_artifact in internal/agent/ext/exhibit.ts; on success it should call the same update_artifact backing call (PATCH the artifact body) so it goes through the existing scoped credential and single-write-path, then behave like update_artifact for scan/footprint purposes (architecture.md §3.7).

## Acceptance Criteria

- edit tool is registered on agent sessions with the same scoping as update_artifact/get_artifact
- exact-match replacement works for a single edit and for multiple non-overlapping edits in one call
- fuzzy fallback normalizes smart quotes, unicode dashes, and whitespace and still applies correctly
- duplicate/non-unique oldText is rejected with a clear error instead of silently editing the first match
- overlapping or touching edits in the same call are rejected
- empty oldText is rejected
- edits are applied in reverse order so offsets don't shift
- untouched lines are preserved byte-for-byte (line endings, BOM) even when fuzzy matching normalized the touched region
- resulting body is persisted the same way update_artifact persists (single write path, scan re-run per architecture.md §3.1)

