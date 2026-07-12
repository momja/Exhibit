-- +goose Up
-- BYO agent API keys (Exh-ky6e): one configured provider key per owner. The
-- key is stored encrypted (AES-GCM under the server secret) — never plaintext.
CREATE TABLE IF NOT EXISTS agent_keys (
    owner_id INTEGER PRIMARY KEY,
    provider TEXT NOT NULL,
    model TEXT NOT NULL DEFAULT '',
    key_ciphertext TEXT NOT NULL,
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

-- Agent conversation transcripts persisted with the artifact they produced
-- (colophon-style provenance, av-q3wo). One row per (artifact, session);
-- messages is the Pi session's message list as JSON.
CREATE TABLE IF NOT EXISTS agent_transcripts (
    artifact_id TEXT NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
    session_id TEXT NOT NULL,
    messages TEXT NOT NULL DEFAULT '[]',
    updated_at DATETIME NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (artifact_id, session_id)
);

-- +goose Down
DROP TABLE IF EXISTS agent_transcripts;
DROP TABLE IF EXISTS agent_keys;
