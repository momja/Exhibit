-- +goose Up
-- Storage accounting learns about out-of-line assets (av-20fk over av-fw1b).
--
-- Migration 021 named this migration before it existed: `blob_references` is
-- the extension point, and replacing the view is the whole change. Every
-- consumer — the per-owner usage query, the recompute pass, the backfill, and
-- ForgetBlobSizes' unreferenced prune — reads the view and none of them knows
-- what a reference is made of, so all four pick the assets up with no code
-- change. That is what the split between blob_sizes (a fact about bytes, no
-- owner) and blob_references (where ownership lives) bought.
--
-- Two things follow that are worth stating outright, because they are the
-- reason this is not optional bookkeeping.
--
-- First, without it a vendored payload is charged to nobody. Those are by far
-- the largest bytes the system stores — a wasm module is most of what a
-- snapshot weighs, which is the entire reason av-20fk moved them out of the
-- body — so an instance would report a library at a small fraction of its size
-- on disk, and the `on disk` line in `server storage usage` would tower over
-- the sum of the per-owner ones for no visible reason.
--
-- Second, and worse, the prune direction: ForgetBlobSizes keeps a length only
-- while `blob_references` still names its blob. With assets missing from the
-- view, deleting one artifact would drop the recorded length of a payload a
-- *second* artifact in the same library still uses, silently shrinking that
-- owner's total until somebody ran a recompute they had no reason to run.
--
-- The asset arm goes through `artifacts` rather than carrying an owner of its
-- own, because artifact_assets has no owner_id: an asset belongs to an
-- artifact, and the artifact is what belongs to somebody. That join is also
-- what keeps the per-owner charge right for a blob two of one owner's
-- artifacts share — the readers take DISTINCT blob_id per owner, so it is
-- counted once for them, and (as before) in full for every other owner that
-- separately references it, since content addressing is deliberately per
-- owner and never global.
DROP VIEW blob_references;

CREATE VIEW blob_references AS
    SELECT source_blob_id AS blob_id, owner_id FROM artifacts WHERE source_blob_id != ''
    UNION ALL
    SELECT widget_blob_id AS blob_id, owner_id FROM artifacts WHERE widget_blob_id != ''
    UNION ALL
    SELECT aa.blob_id AS blob_id, a.owner_id AS owner_id
      FROM artifact_assets aa
      JOIN artifacts a ON a.id = aa.artifact_id
     WHERE aa.blob_id != '';

-- +goose Down
DROP VIEW blob_references;

CREATE VIEW blob_references AS
    SELECT source_blob_id AS blob_id, owner_id FROM artifacts WHERE source_blob_id != ''
    UNION ALL
    SELECT widget_blob_id AS blob_id, owner_id FROM artifacts WHERE widget_blob_id != '';
