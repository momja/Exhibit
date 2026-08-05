-- +goose Up
-- Per-owner agent preferences: non-secret settings the agent surface applies
-- to every session it spawns for that owner. Kept separate from agent_keys
-- (which holds only the encrypted provider credential) so a preference edit
-- never touches the row carrying key_ciphertext.
CREATE TABLE IF NOT EXISTS agent_config (
    owner_id INTEGER PRIMARY KEY,
    system_prompt TEXT NOT NULL DEFAULT '',
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

-- +goose Down
DROP TABLE IF EXISTS agent_config;
