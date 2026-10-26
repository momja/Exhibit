---
id: Exh-77fs
status: in_progress
deps: []
links: []
created: 2026-10-26T00:13:51Z
type: chore
priority: 3
assignee: Max Omdal
tags: [agent, cleanup]
---
# Remove redundant copy before agent event publication

handleLine clones an incoming Pi event before redaction, then broadcast clones it again before retaining and publishing it. Redaction does not mutate its input. Remove only the caller-side clone so the existing publication module remains responsible for byte ownership.

## Acceptance Criteria

Remove only the clone before redaction. Preserve the retained-byte clone in broadcast, hosted redaction, replay ordering and queue bounds. Run the existing event ownership and redaction tests; make no lifecycle or interface changes.

