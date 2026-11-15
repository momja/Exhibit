-- +goose Up
-- Whether agents may read and change an artifact's saved data (Exh-dabd).
-- Agents run on an inference provider, so a state tool is a way for what a
-- person typed into their tools to leave the instance. Both halves default to
-- no: the account default to off, and every artifact to following it.
--
-- owner_settings is keyed by owner_id with no users foreign key, because a
-- no-login owner has no users row (the same reasoning as agent_grants).
-- Account deletion removes it explicitly.
CREATE TABLE owner_settings (
    owner_id           INTEGER PRIMARY KEY,
    agent_state_access INTEGER NOT NULL DEFAULT 0 CHECK (agent_state_access IN (0, 1)),
    updated_at         DATETIME NOT NULL DEFAULT (datetime('now'))
);

ALTER TABLE artifacts ADD COLUMN agent_state_access TEXT NOT NULL DEFAULT 'inherit'
    CHECK (agent_state_access IN ('inherit', 'allow', 'deny'));

-- +goose Down
ALTER TABLE artifacts DROP COLUMN agent_state_access;
DROP TABLE IF EXISTS owner_settings;
