-- +goose Up
-- An agent can request setup, never authorize it. These rows contain only the
-- provider id and the explanation shown in app chrome. Static-token instances
-- have owner 1 without a users row, so artifact deletion supplies the cascade.
CREATE TABLE artifact_connection_requests (
    id TEXT PRIMARY KEY,
    owner_id INTEGER NOT NULL,
    artifact_id TEXT NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
    provider TEXT NOT NULL,
    reason TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE(artifact_id, provider)
);
CREATE INDEX artifact_connection_requests_owner ON artifact_connection_requests(owner_id, artifact_id);

-- +goose Down
DROP TABLE IF EXISTS artifact_connection_requests;
