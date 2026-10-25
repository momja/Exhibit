---
id: Exh-tjib
status: closed
deps: [Exh-grj9]
links: []
created: 2026-10-25T18:10:48Z
type: task
priority: 2
assignee: Max Omdal
parent: Exh-35gj
tags: [store, performance, migration]
---
# Owner scoping without a full artifacts scan: EXISTS ownedArtifact + artifacts(owner_id) index

`ownedArtifact` (internal/store/sqlite.go:606) is `artifact_id IN (SELECT id FROM artifacts WHERE owner_id=?)`, interpolated into ~25 per-artifact queries (sqlite.go, versions.go, transcripts.go, assets.go). artifacts has no owner_id index, so each one plans as LIST SUBQUERY / SCAN artifacts — reading every artifact on the instance to check one. ListArtifacts (WHERE a.owner_id = ?) scans too.

Scope:
- Replace the IN form with a correlated primary-key check, `EXISTS (SELECT 1 FROM artifacts WHERE artifacts.id = <tbl>.artifact_id AND artifacts.owner_id = ?)`. It must name the outer table, so it becomes a small helper; keep it one definition and keep the semantics identical (owner_scope_test.go and the AGrantDoesNotWidenAnyOwnerScopedMethod test stay green unchanged).
- Migration (next free number after story 2's, i.e. 039 if story 2 takes 038) adding `CREATE INDEX artifacts_owner ON artifacts(owner_id)` for the gallery listing.
- A test asserting the plans: per-artifact child queries SEARCH artifacts by primary key; ListArtifacts uses artifacts_owner.

