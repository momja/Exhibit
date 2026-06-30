-- +goose Up
ALTER TABLE artifacts ADD COLUMN source_url TEXT NOT NULL DEFAULT '';

-- +goose Down
-- SQLite cannot easily DROP a column without recreating the table; leave as a no-op.
