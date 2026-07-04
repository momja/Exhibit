---
id: exhibit-x87
status: open
deps: []
links: []
created: 2026-07-01T06:02:26Z
type: task
priority: 2
---
# Model network origin decisions in artifact_network_origins table (replace network_allowlist JSON)

Pre-v1 decision (from the exhibit-i0k discussion): replace the network_allowlist JSON-array column on artifacts with a relational child table, matching the existing artifact_state / tags / collections idiom and enforcing invariants in the schema. Foundation for the runtime blocklist (exhibit-fr7). Schema: artifact_network_origins(artifact_id TEXT REFERENCES artifacts(id) ON DELETE CASCADE, origin TEXT, decision TEXT CHECK(decision IN ('allow','block')), source TEXT, created_at, updated_at, PRIMARY KEY(artifact_id, origin)). decision='allow' rows drive the render CSP (connect/script/img/style/font-src); decision='block' rows are 'do not re-prompt' markers consumed by the runtime prompt and NEVER affect the CSP. The PK enforces one decision per origin; ON DELETE CASCADE removes rows when the artifact is deleted.

## Design

- goose migration: create table; backfill existing network_allowlist arrays as decision='allow', source='legacy'; drop the network_allowlist column (pre-v1, negligible data). - Store interface: ListOriginDecisions(artifactID); SetOriginDecision(artifactID, origin, decision, source) as UPSERT; DeleteOriginDecision(artifactID, origin); AllowedOrigins(artifactID) helper for CSP. Keep the Artifact response shape working (hydrate a computed allowlist for API back-compat) or move callers to decisions. - render.buildCSP: build from decision='allow' rows. - API: keep PATCH accepting network_allowlist:[...] (translate to an allow-row replacement) so the shipped ingest approval + detail-page editor keep working, OR add a per-origin decisions endpoint; prefer minimal churn to the single write path. - Update createArtifact approval writes, updateArtifact, gallery detail allowlist editor, and gallery ingest approval to read/write via the table. - Latent option (do NOT build now): a per-directive capability column so approving a connect target need not also grant script execution — the JSON array cannot express this; the table keeps the door open.

## Acceptance Criteria

artifacts has no network_allowlist column; decisions live in artifact_network_origins. Render CSP is generated from decision='allow' rows and matches prior behavior for migrated artifacts. Ingest approval and detail-page add/remove still work end to end. Duplicate (artifact,origin) is prevented by the PK (upsert). Tests: allow-row drives CSP; block rows never affect CSP; upsert/delete; cascade on artifact delete.


