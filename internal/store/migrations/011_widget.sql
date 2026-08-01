-- +goose Up
-- Artifact widgets (av-fafu): an artifact may carry a second self-contained
-- HTML document — the glanceable tile its gallery card renders. It is a body,
-- not metadata, so it lives in the blob store beside the artifact's own source
-- and the row keeps only its id. Empty means "no widget", which is the default
-- and makes the card fall back to the server-rendered default tile.
--
-- Deliberately NOT a second artifacts row: a widget has no independent
-- identity, no allowlist of its own, and no state of its own. It reads the
-- owning artifact's state and renders under the owning artifact's CSP, so
-- modelling it as a column keeps that one-to-one binding a schema fact rather
-- than a convention two tables have to agree on.
ALTER TABLE artifacts ADD COLUMN widget_blob_id TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE artifacts DROP COLUMN widget_blob_id;
