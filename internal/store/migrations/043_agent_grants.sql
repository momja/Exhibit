-- +goose Up
-- Exh-l2xj: owner-issued credentials for agents outside the server. Each row
-- is authority over exactly one artifact, the durable counterpart of the
-- process-lifetime grants agentscope mints for Pi. Only the SHA-256 of the
-- token is kept: the plaintext is shown to the owner once, at issue.
--
-- No expiry column, on the reasoning that removed the one shares had (av-8ipt):
-- a grant lives until its owner revokes it, and what keeps a forgotten one in
-- view is the artifact's list of them rather than a timer.
--
-- owner_id is not a foreign key, like artifacts.owner_id: a single-user
-- instance has no users row for owner 1. The artifact cascade is what removes
-- a grant with its library, and resolution re-checks that the artifact still
-- belongs to the grant's owner and that the account is not disabled.
CREATE TABLE agent_grants (
    id TEXT PRIMARY KEY,
    token_hash TEXT NOT NULL UNIQUE,
    owner_id INTEGER NOT NULL,
    artifact_id TEXT NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
    label TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    last_used_at TEXT
);
CREATE INDEX agent_grants_by_artifact ON agent_grants(artifact_id);

-- +goose Down
DROP TABLE agent_grants;
