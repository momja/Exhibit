---
id: Exh-08e0
status: closed
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


## Notes

**2026-10-24T04:44:46Z**

Confirmed the recent 403s were the ticket visualizer's optional POST to https://api.github.com/graphql. The operation was a read-only query, but its native Bearer connection has allow_writes=false and therefore permits GET/HEAD only. The independent relay write grant had already been approved; that does not expand the credential's capabilities. REST GETs succeeded. No provider secret or permission change was needed.

Repaired only the artifact's _loadGraphQL method: inspect window.exhibit.connections.list(), require an active connection for the exact GitHub origin with write capability before attempting the optional POST, and return null to the existing REST fallback for read-only, missing, reconnecting or unavailable metadata. A write-capable connection on another origin cannot enable it. Applied the source with expected_source_revision, from 495848082b5829db082cad8ee21007158dd7a1996bf037a5a5dbaabc1c3d3a0d to version 3 / 6c7def0b0121fef4892d8af4be561d7160d6dbf7db410cb49c43f41dd0023f53. No concurrent source edit was overwritten. The full source is private to the artifact; no artifact source or secret was committed to the repository.

Validated the actual draft and saved method with nine Node VM checks: reproduce the original unconditional POST, skip POST for the read-only connection, reject write authority from another origin, preserve the bulk query for an active matching write-capable connection, and safely fall back for missing/reconnecting/failed/malformed metadata or standalone execution. The saved method exactly matches the tested repair. Model prompt tests, go vet ./internal/agent and the server build passed. Updated the model guidance and operator docs to state that read-only means GET/HEAD and GraphQL queries sent as POST need both credential and relay write permission; use GET alternatives for optional optimizations.

Rebuilt/restarted the experimental demo with the guidance update and verified the refreshed artifact: recent live traffic contained 91 successful GETs, no 403 resource in the inspected page, and the existing GitHub connection remains active with allow_writes=false. Credentials, grants and saved data were retained. Users with an already-open page must reload to use source version 3.
