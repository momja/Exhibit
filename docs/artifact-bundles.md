# Artifact bundles

Implementation contract for the portable artifact epic. The complete download
is a ZIP from `GET /api/artifacts/:id/export` (also `?format=zip`). An explicit
`?format=html` retains the single self-contained HTML document. Other formats
are rejected. Both are owner-only downloads.

`manifest.json` has `format: "exhibit.artifact"`, `version: 1`, source
`artifact_id` and `render_origin`, portable `artifact` metadata, `entry_page`, a
`pages` array, optional `widget`, `state_path`, and `assets`. The types in
`internal/api/artifact_bundle_types.go` are the shared schema. Each page has a
logical `id`, a self-contained HTML `path`, and a separate editable `source_path`.
The initial exporter writes one page (`main`) and preserves this distinction so
restoration does not have to reverse materialization. The current importer must
reject more than one page until the product supports them.

State is a flat JSON object of owner string values, using the existing Stored
state format. Assets reuse the Stored assets manifest row (including `path`)
and contain original bytes. Current assets plus historical assets still reached
by the current source/widget/textual assets must be retained. No archive path is
derived from a source URL without sanitization. Every named file is unique, and
the ZIP has exactly the entries named by its manifest plus the manifest itself.

Bundles transfer the current artifact, widget, state, and assets. Version history,
account credentials, shares, chats, and account-specific tag/collection membership
are outside this snapshot. Metadata records source title/URL/tier and approvals;
an imported artifact starts private with fresh network and capability approvals.

The private import is `POST /api/admin/users/:userID/artifacts/import`, with the
raw ZIP as its request body. It is disabled unless
`ARTIFACT_IMPORT_ADMIN_TOKEN` is configured and requires that exact token in an
Authorization Bearer header. An ordinary service token or admin session does not
authorize import. Refusals return 404 before target lookup or body parsing. No
gallery/admin controls or public API documentation advertise this endpoint.
Operator details belong in this document and deployment configuration only.

Import creates a new artifact, returns 201 with its ordinary artifact JSON, and
never overwrites an existing artifact. It validates manifest version, all paths,
file types, counts, expanded sizes, CRCs, metadata and state before staging any
blob. Reuse `readImportEntry`, `readStateImport` and the staging/quota/cleanup
pattern from Stored data import. All source/widget/state/assets rows publish in
one transaction. New artifact and asset identities are generated and references
to the source render origin are rebound in source, widget, textual assets and
runtime source URLs. Never rebind an old immutable public asset identity.
