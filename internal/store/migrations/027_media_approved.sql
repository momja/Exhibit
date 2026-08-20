-- +goose Up
-- Per-artifact first-use camera and microphone approval (av-mv3k). 0 = not
-- approved. Two columns rather than one "media_approved": a tool that wants a
-- microphone for dictation must not acquire a camera along the way, and the
-- host prompts for exactly the devices the artifact's getUserMedia constraints
-- asked for.
--
-- Fourth and fifth siblings of downloads_approved (005), clipboard_approved
-- (006) and links_approved (018) — the same per-artifact first-use decision,
-- persisted the same way through PATCH /api/artifacts/:id. Unlike those three
-- these two are enforced on a top-level render rather than delivered in the
-- frame: they build the render document's Permissions-Policy header, which is
-- what makes the grant per-artifact rather than per-render-origin (a browser
-- permission granted to one artifact opened directly would otherwise be
-- inherited by every other artifact on that origin). Neither touches the CSP.
ALTER TABLE artifacts ADD COLUMN camera_approved INTEGER NOT NULL DEFAULT 0;
ALTER TABLE artifacts ADD COLUMN microphone_approved INTEGER NOT NULL DEFAULT 0;

-- +goose Down
-- SQLite cannot easily DROP a column without recreating the table; leave as a no-op.
