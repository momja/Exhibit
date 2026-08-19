-- +goose Up
-- Per-owner storage accounting (av-fw1b). This is the first byte count
-- anywhere in the schema: across the twenty migrations before it no table
-- records how large an artifact is, so the only way to answer "how much is
-- this owner holding" was to stat the blob directory — which knows nothing
-- about owners, and on an object-store backend (av-52ll) is a paginated
-- network crawl.
--
-- Two objects, and the split between them is the whole design.
--
-- blob_sizes is a fact about *bytes* and carries no owner. A blob's length is
-- a property of the blob, not of whoever happens to reference it, and putting
-- an owner here would have to be rewritten every time a reference moved —
-- which is exactly what refcounted shared assets (av-20fk) will do.
--
-- blob_references is where ownership lives: one row per (blob, owner) the
-- schema can name. An owner's total is the join of the two, so the total is
-- derived on read and there is no counter to drift out of step with the rows.
-- Deleting an artifact drops its references and its bytes stop being charged
-- in the same statement; nothing has to remember to decrement anything.
--
-- The view is also the extension point. When av-20fk lands its refcounted
-- artifact_assets, a migration REPLACES this view with one that UNION ALLs the
-- asset references in — and every consumer (the usage query, the recompute
-- pass, the unreferenced-size prune) picks the assets up with no code change,
-- because none of them knows what a reference is made of.
--
-- Shared assets are charged at FULL SIZE to EVERY referencing owner. The
-- charge is deduplicated within one owner (the readers take DISTINCT blob_id
-- per owner) and never across owners. That is what makes the number
-- ungameable — an owner cannot shrink their bill by uploading a file another
-- tenant already has — and stable: an owner's total never moves because a
-- stranger deleted something. Rationale in architecture.md §3.3.
CREATE TABLE blob_sizes (
    blob_id    TEXT PRIMARY KEY,
    bytes      INTEGER NOT NULL,
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE VIEW blob_references AS
    SELECT source_blob_id AS blob_id, owner_id FROM artifacts WHERE source_blob_id != ''
    UNION ALL
    SELECT widget_blob_id AS blob_id, owner_id FROM artifacts WHERE widget_blob_id != '';

-- +goose Down
DROP VIEW blob_references;
DROP TABLE blob_sizes;
