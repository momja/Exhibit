-- +goose Up
-- Exh-61ix: which component kit styles an artifact is chosen outside its
-- source. An artifact's own choice overrides its owner's library choice; an
-- empty string means "no choice here, ask the next level".
ALTER TABLE artifacts ADD COLUMN kit TEXT NOT NULL DEFAULT '';
-- Keyed by owner rather than hung on users: an instance with no login has an
-- owner and no users row.
CREATE TABLE library_settings (
    owner_id INTEGER PRIMARY KEY,
    kit TEXT NOT NULL DEFAULT ''
);

-- +goose Down
DROP TABLE library_settings;
ALTER TABLE artifacts DROP COLUMN kit;
