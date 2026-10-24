---
id: Exh-s47o
status: closed
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


## Notes

**2026-10-24T04:29:39Z**

Implemented the additive model across Exhibit and both portable runners. The artifact allowlist remains authoritative for every network destination. Connector grants select a credential only for the matching origin; multiple distinct connections and standalone APIs/CDNs coexist. Retained destination lists and permitted_egress/data_origins are legacy metadata, excluded from both network checks and authority hashes. Existing approvals, credentials and saved state need no migration, cleanup or reconnection.

Removed the global response-data ceiling, its grant controls, the origin-approval guard and the misleading connection-block dialog. Configuration now describes credential destinations and separate network approvals; optional saved-data cleanup, explicit sharing consent, opaque private frames and current document authority remain. Disabling connectors only blocks a matching credentialed request. Public HTTP caches and each portable connection cache remain independent of unrelated connection changes. Native and vendor engines retain origin-bound credentials, stand-in misuse refusal, quotas/cache and redirect checks. New exports omit the legacy data-origins field.

Validation passed on the final tree: go test -json -timeout 15m ./... (24 packages passed, 7 optional/no-test packages skipped, zero failures), go vet ./..., all 391 gallery tests, full connector race suite, full Go/Node bundle race suite and focused API race tests covering additive requests, independent public caching, origin approval, navigation, retained provenance and current authority. Chromium covers the real renderer/CSP and viewer/agent preview with two connections plus an approved stylesheet, and approval of a new origin while connections remain attached. A browser-test navigation wait was corrected and passed five consecutive repetitions plus the full run.

Rebuilt and restarted the experimental demo. The reported artifact loads its previously approved unpkg.com icons with no network/protection notice while GitHub relay metadata shows successful 200 requests. Its saved origin approvals and existing account/state data were retained. Updated configuration and mobile views have no legacy destination control or overflow. Screenshots are in docs/screenshots/Exh-s47o; obsolete protection-dialog screenshots were removed. Architecture, security, PRD, API, operator docs, model guidance and example README now use the additive model. Prior ticket notes record that the user's clarification supersedes the old response-data ceiling. The retired-provider tracked-file search remains empty.
