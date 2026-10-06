-- +goose Up
-- Per-artifact first-use geolocation approval (av-f446). 0 = not approved.
--
-- The sixth sibling of downloads_approved (005), clipboard_approved (006),
-- links_approved (018) and camera_approved / microphone_approved (027), and
-- shaped like the last two rather than the first three: it is enforced on a
-- top-level render, where it builds the document's Permissions-Policy header,
-- rather than spent by a host bridge in the frame. A browser's location grant
-- belongs to an origin and every artifact shares one render origin, so without
-- the header a visitor who allowed location for one artifact opened directly
-- would have allowed it for all of them. It does not touch the CSP.
ALTER TABLE artifacts ADD COLUMN geolocation_approved INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE artifacts DROP COLUMN geolocation_approved;
