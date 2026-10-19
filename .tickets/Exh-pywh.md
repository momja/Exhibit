---
id: Exh-pywh
status: in_progress
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

