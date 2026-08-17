-- +goose Up
-- Per-artifact first-use external-link approval (av-r0dk). 0 = not approved:
-- the host frame prompts on the artifact's first external-link click and
-- persists the user's approval here so it survives reloads and devices.
-- Sibling to downloads_approved (005) and clipboard_approved (006) — the same
-- host-mediated capability bridge, this time for navigation instead of an API.
ALTER TABLE artifacts ADD COLUMN links_approved INTEGER NOT NULL DEFAULT 0;

-- +goose Down
-- SQLite cannot easily DROP a column without recreating the table; leave as a no-op.
