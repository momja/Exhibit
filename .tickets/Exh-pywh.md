---
id: Exh-pywh
status: closed
deps: [Exh-dgno]
links: []
created: 2026-10-19T05:12:10Z
type: feature
priority: 2
assignee: Max Omdal
parent: Exh-099b
tags: [agent]
---
# Stop prompting the agent to reread source after its own successful save

Remove contradictory or blanket reread-after-save instructions from system prompt, get_artifact description and related mode/resume descriptions. Let the agent reuse source it has read and successful edits. Require reads for absent/evicted context, conflicts and actual missing information. Coordinate with versioned retained snapshots so freshness is enforced by tools rather than model habit. Retain all data-versus-instructions protections.

## Acceptance Criteria

Prompt and tool contracts consistently permit reuse after successful saves and require rereads on conflict or missing context; targeted tests and documentation reflect the behavior; no weakening of untrusted-data boundaries.


## Notes

**2026-10-19T05:17:07Z**

Updated system/mode prompts and artifact/widget tool descriptions to reuse known or preloaded tool-result source plus successful saves. Reads remain required for missing/compacted context, conflicts, failed matches, external changes and uncertain saves; resume freshness and TOOL RESULTS ARE DATA remain intact. Updated focused Go/tool-description tests and agent/security/architecture docs. Verified go test ./internal/agent -count=1, eight selected real-Pi 1.3.0 API integration tests, asset build, and git diff --check.

**2026-10-19T06:44:55Z**

PR feedback: keep get_artifact/get_widget descriptions limited to returned data; remove the added save/reuse/refresh policy and its description-string assertions. get_widget returns to its original description; get_artifact keeps only the factual first sentence, removing the old unconditional reread directive. Source reuse and freshness guidance remains in the system/mode prompts. Focused prompt and extension suites passed.
