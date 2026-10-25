---
id: Exh-35gj
status: in_progress
deps: []
links: [av-0k5q]
created: 2026-10-25T18:10:48Z
type: epic
priority: 2
assignee: Max Omdal
tags: [sharing, store, schema]
---
# Split shares into links and grants (prerequisites for 'Shared with me')

Prerequisites for the 'Shared with me' gallery space (av-0k5q, B2 navigation: spaces in a bottom tab bar on phones / rail on desktop, filters in a sheet).

Today one `shares` table holds two different things, told apart only by a nullable `recipient_id`: NULL is the artifact's anonymous public link (/s/:id, the random id is the authorization), set is a grant to one account (opens /artifacts/:id, the id is only a row key). Consequences found in design review (2026-10-25):

- Fail-open default: `store.Share.RecipientID` is `*int64` and nil means 'anonymous link', so a grant path that forgets the recipient silently makes the artifact public (only caught by the partial unique index if a link already exists).
- Grant ids look like link secrets; only `AND recipient_id IS NULL` in GetAnonymousShareUnscoped keeps them from working at /s/:id.
- No index leads with recipient_id, so listing 'shared with me' scans every share row (EXPLAIN QUERY PLAN: SCAN s USING COVERING INDEX shares_artifact_recipient).
- `artifacts.share_link` / `share_grant_count` are trigger-maintained rollups (migration 029) for the card badge; with 24-card paging (av-kyhl) an EXISTS per card over an indexed table is cheap, and the triggers would have to be rewritten for any split anyway.
- Separately: `ownedArtifact` (sqlite.go:606) is `artifact_id IN (SELECT id FROM artifacts WHERE owner_id=?)`, and artifacts has no owner_id index, so ~25 per-artifact queries SCAN artifacts.

Stories, stacked in this order (one PR each):
1. Store API split: CreateGrant / CreateAnonymousLink, revoke grants by (artifact, recipient). No migration.
2. Schema split: share_links + share_grants (with recipient index), data migration, drop shares and the rollup columns/triggers, badge via EXISTS.
3. Ownership-check performance: ownedArtifact as correlated EXISTS + artifacts(owner_id) index.

