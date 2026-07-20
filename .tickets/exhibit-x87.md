---
id: exhibit-x87
status: in_progress
deps: []
links: [av-jafp, av-p0a1]
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



## Notes

**2026-07-20T02:11:47Z**

Linked to av-jafp / av-p0a1 (merged in PR #61, branch feature/av-jafp/epic):
the Edit page's "Referenced, not approved" section is built directly on
today's network_allowlist shape and needs explicit attention in this
migration, not just "keep it working":

- internal/api/gallery.go renderEditPage() reads `a.NetworkAllowlist`
  directly and computes `unapproved := diffOrigins(scanner.Scan(src),
  allowlist)` (diffOrigins ~line 269) — a plain "is this origin in the
  allowed array" check. Once origins have three states (allow / block /
  undecided) instead of two (allowlisted / not), this binary diff is no
  longer sufficient: an origin scanner.Scan finds that already has an
  explicit decision='block' row (from the future runtime prompt's "don't
  ask again", exhibit-fr7) would currently render identically to one
  that's simply never been decided — both show up as a plain "Allow" row.
  Decide and implement: does the Edit page show blocked origins in this
  list at all, and if so, does it label them distinctly (e.g. "blocked —
  Allow" to permit an override) rather than silently offering to allow
  them like any other undecided origin?
- renderEditPage must switch from `a.NetworkAllowlist` to whatever read
  path this ticket lands on (AllowedOrigins(artifactID) per the design
  above, or the hydrated-Artifact.NetworkAllowlist compat shim if that's
  the chosen approach) — same for the gallery card badge (av-isb3) and
  popover (av-41se), which also read NetworkAllowlist for their counts/
  origin lists.
- The Edit page's single Save does one PATCH carrying the whole working
  `network_allowlist` array (web/gallery/edit.js, internal/api/edit.tmpl
  "Add origin"/"Allow" rows). If PATCH's translation to per-origin
  allow-row upserts is implemented as "replace all of this artifact's
  decisions with the PATCHed array," it must only touch decision='allow'
  rows — an Edit-page Save must never silently clear existing
  decision='block' rows it doesn't know about.
- Test coverage to add alongside this ticket's existing acceptance
  criteria: an origin with an existing block decision still renders (in
  whatever form is decided above) rather than disappearing or crashing;
  Edit-page Save doesn't delete unrelated block rows.
