-- +goose Up
ALTER TABLE tags ADD COLUMN color TEXT NOT NULL DEFAULT '#6B7280';

-- +goose Down
ALTER TABLE tags DROP COLUMN color;
