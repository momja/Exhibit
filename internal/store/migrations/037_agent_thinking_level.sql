-- +goose Up
-- Empty preferences follow the instance baseline, including existing keys.
ALTER TABLE agent_keys ADD COLUMN thinking_level TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE agent_keys DROP COLUMN thinking_level;
