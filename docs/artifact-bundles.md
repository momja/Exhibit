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

Reference parsing allows at most 32 context levels and cumulative input of eight
times each source/payload's length plus 8 KiB. Nested `srcdoc` documents share
their enclosing document's budget. Exhaustion rejects export before download
headers or import before staging.

## Operator restore

Set `ARTIFACT_IMPORT_ADMIN_TOKEN` to a dedicated secret on the destination
server, then restart it. Keep this separate from `AUTH_TOKEN`; service tokens,
admin sessions, and ordinary sessions cannot authorize bundle imports. Leaving
the variable empty disables the endpoint. The configured secret is accepted
only through `Authorization: Bearer …`, never through a query parameter.

Provision the destination account first and use its integer user ID. It must
exist and remain enabled through publication. Send the exported ZIP directly,
without multipart encoding:

```sh
curl --fail-with-body \
  -H "Authorization: Bearer $ARTIFACT_IMPORT_ADMIN_TOKEN" \
  -H 'Content-Type: application/zip' \
  --data-binary @artifact.zip \
  https://exhibit.example.com/api/admin/users/42/artifacts/import
```

Success returns `201` and the ordinary artifact JSON, including its fresh ID.
Every request creates a new private artifact; repeating a successful request
creates another copy. The original title, source URL, tier, editable source,
widget, asset bytes, and exact owner-state strings travel. Source artifact,
asset, and generation identities are fresh, as are source and widget blob IDs. Asset blobs use the destination account’s content addresses
so identical payloads share their storage and quota charge. Network decisions and
capability approvals reset, sharing defaults to `own`, and no shares or account
organization are copied. Portable HTML entries are validated but the separate
editable source entries are restored.

The ZIP upload uses `MAX_REQUEST_BODY_BYTES` (32 MiB by default), including
chunked requests. Each expanded page, editable source, widget, and state file
uses that same ceiling. The manifest is at most 1 MiB. At most 200 asset rows
are accepted, with at most 96 MiB of expanded asset entries and 48 MiB of unique
asset payloads, including historical dependencies. Owner state is at most
10,000 string keys. These limits also apply when exporting a complete bundle.
An unsupported format/version, multiple pages, missing references, malformed
metadata/state, unsafe or duplicate paths, unlisted files, or a bad CRC fails
before any blob is staged. Destination storage quotas apply to the rewritten
source/widget and distinct asset payloads after rewriting.

A missing or wrong dedicated token returns the same `404` before account
lookup or request-body inspection, including for declared oversized uploads.
With the correct token, a nonexistent/disabled account returns `404`, invalid
archives return `400`, and an upload/quota/state-key limit returns `413`.
Expanded entry and asset-budget violations return `400`. Publication uses one
database transaction: failed publication exposes no artifact, source/widget
version, assets, or state. Staged blobs are durably queued before writes and
reclaimed after a failure; startup cleanup retries any interrupted reclamation.
