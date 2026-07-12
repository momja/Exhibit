-- +goose Up
-- Per-artifact first-use clipboard approval (av-hll6). 0 = not approved: the
-- host frame prompts on the artifact's first navigator.clipboard read/write
-- and persists the user's approval here so it survives reloads and devices.
-- Sibling to downloads_approved (005) — same host-mediated capability bridge.
ALTER TABLE artifacts ADD COLUMN clipboard_approved INTEGER NOT NULL DEFAULT 0;

-- +goose Down
-- SQLite cannot easily DROP a column without recreating the table; leave as a no-op.
