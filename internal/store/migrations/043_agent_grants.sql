-- +goose Up
-- Owner-issued library credentials. The random plaintext token is shown only
-- when issued; only its SHA-256 digest is persisted. Grants live until revoked.
-- owner_id deliberately has no users FK, like artifacts.owner_id: no-login
-- instances have no users row. DeleteAccount explicitly removes these rows.
CREATE TABLE agent_grants (
    id TEXT PRIMARY KEY,
    token_hash TEXT NOT NULL UNIQUE,
    owner_id INTEGER NOT NULL,
    label TEXT NOT NULL,
    client TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    last_used_at TEXT
);
CREATE INDEX agent_grants_by_owner ON agent_grants(owner_id);

-- +goose Down
DROP TABLE agent_grants;
