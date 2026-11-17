-- +goose Up
ALTER TABLE artifacts ADD COLUMN share_attribution TEXT NOT NULL DEFAULT 'inherit' CHECK (share_attribution IN ('inherit', 'show', 'hide'));

-- +goose Down
ALTER TABLE artifacts DROP COLUMN share_attribution;
