---
id: Exh-s47o
status: in_progress
deps: []
links: []
created: 2026-10-24T04:03:10Z
type: bug
priority: 2
assignee: Max Omdal
parent: Exh-g2vv
tags: [experimental, connectors, permissions]
---
# [Experimental] Make connector grants additive to artifact network permissions

User clarification: connectors are additive. One artifact may use multiple independent connectors and approved standalone origins with no connector. A connection must not intersect or narrow unrelated network permissions. Keep artifact origin approval as the network boundary and credential injection scoped to the matching connection origin; preserve no cross-origin credential forwarding, grants, sharing, quotas and revocation. Remove the misleading response-data destination ceiling, its configuration controls and the follow-up connection-block notice. Existing approvals and saved state must work without cleanup or reconnection.

## Acceptance Criteria

An artifact with two distinct-origin connections can call both and an approved unauthenticated API/CDN. Requests use only the matching credential, standalone requests use none, unapproved origins and wrong-origin stand-ins remain blocked. Adding, removing, revoking or disabling a connector does not narrow unrelated approved traffic or require clearing saved data. Viewer/agent CSP, relay, schedules and portable Go/Node exports agree; normal origin approval works; UI/docs describe additive permissions and credentials remain secret. Cover active and legacy retained-policy records, shared ownership and redirects with meaningful tests.

