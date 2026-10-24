---
id: Exh-08e0
status: in_progress
deps: []
links: []
created: 2026-10-24T04:38:07Z
type: bug
priority: 2
assignee: Max Omdal
parent: Exh-g2vv
tags: [experimental, connectors, artifact]
---
# [Experimental] Avoid unsupported GraphQL POST with read-only connections

The ticket visualizer sends an optional GitHub GraphQL query using POST before its REST fallback. Its saved native Bearer connection is read-only (GET/HEAD), so this probe returns 403 even after the independent relay write grant is approved. Check the runtime connection capabilities before attempting GraphQL, skip unsupported POSTs and retain the REST GET path without expanding credential permissions. Teach artifact agents the current HTTP-method meaning of read-only access.

## Acceptance Criteria

The existing read-only GitHub connection uses REST without an avoidable GraphQL POST/403; matching write-capable connections may retain the batch query; unrelated connections never enable it; absent/unavailable connection metadata degrades to GET fallback. Preserve source, state, grants and credentials except the narrowly scoped source repair, and protect against concurrent source edits. Verify the actual artifact code, update model guidance/docs and push the branch.

