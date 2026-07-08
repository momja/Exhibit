-- +goose Up
-- Per-artifact first-use download approval (av-ryby). 0 = not approved: the
-- host frame prompts on the artifact's first download attempt and persists
-- the user's approval here so it survives reloads and devices.
ALTER TABLE artifacts ADD COLUMN downloads_approved INTEGER NOT NULL DEFAULT 0;

-- +goose Down
-- SQLite cannot easily DROP a column without recreating the table; leave as a no-op.
