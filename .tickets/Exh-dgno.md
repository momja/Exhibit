---
id: Exh-dgno
status: closed
deps: [av-n0gl]
links: []
created: 2026-10-19T05:12:10Z
type: feature
priority: 2
assignee: Max Omdal
parent: Exh-099b
tags: [agent]
---
# Reuse source snapshots and send patches from artifact and widget edit tools

Refactor edit_artifact and edit_widget to retain the exact source/revision read, apply validated edits locally or through a scoped server patch endpoint, and avoid fetching and submitting the whole document for every targeted edit. Prefer a clean patch API if needed to avoid full-body tool-to-API traffic; coordinate with OCC and eager context loading. Successful saves advance the snapshot/revision, conflicts never silently rebase, and resumed/compacted context cannot use a mismatched base. Keep full rewrite tools for deliberate replacements.

## Acceptance Criteria

A targeted edit uses known versioned source and patch payloads rather than full-source GET plus full-body replacement each call. Ambiguous/overlapping/missing matches and stale bases save nothing; success advances the known snapshot; artifact and widget behavior and existing scan/version/scope paths are covered.


## Notes

**2026-10-19T05:33:53Z**

Implemented retained source/widget snapshots, exact patch payloads through existing write routes, revision-bound full rewrites, and explicit conflict invalidation. Validation: 31 extension tests; patch API and malformed/no-op tests; real-Pi successive edits with one source read; scoped agent/guardrail/conversation/resume integration checks passed.
