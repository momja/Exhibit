-- +goose Up
-- Credentials and cached provider data are sealed by secrets.Box. owner_id and
-- viewer_id deliberately support single-user installations without a users row.
CREATE TABLE IF NOT EXISTS connections (
 id TEXT PRIMARY KEY,
 owner_id INTEGER NOT NULL,
 name TEXT NOT NULL,
 provider TEXT NOT NULL,
 kind TEXT NOT NULL CHECK(kind IN ('native','oauth','vendor')),
 recipe_json TEXT NOT NULL,
 encrypted_secret TEXT NOT NULL,
 status TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active','revoked','reconnect')),
 created_at DATETIME NOT NULL DEFAULT (datetime('now')),
 last_used_at DATETIME
);
CREATE INDEX IF NOT EXISTS connections_owner ON connections(owner_id, kind, status);
CREATE TABLE IF NOT EXISTS artifact_connection_grants (
 artifact_id TEXT NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
 connection_id TEXT NOT NULL REFERENCES connections(id) ON DELETE CASCADE,
 viewer_id INTEGER NOT NULL,
 permitted_egress_json TEXT NOT NULL DEFAULT '[]',
 share_with_recipients INTEGER NOT NULL DEFAULT 0 CHECK(share_with_recipients IN (0,1)),
 created_at DATETIME NOT NULL DEFAULT (datetime('now')),
 PRIMARY KEY(artifact_id,connection_id,viewer_id)
);
CREATE INDEX IF NOT EXISTS connection_grants_viewer ON artifact_connection_grants(viewer_id,artifact_id);
CREATE TABLE IF NOT EXISTS connection_usage (
 connection_id TEXT NOT NULL REFERENCES connections(id) ON DELETE CASCADE,
 day TEXT NOT NULL,
 requests INTEGER NOT NULL DEFAULT 0 CHECK(requests >= 0),
 PRIMARY KEY(connection_id,day)
);
CREATE TABLE IF NOT EXISTS connection_cache (
 connection_id TEXT NOT NULL REFERENCES connections(id) ON DELETE CASCADE,
 request_hash TEXT NOT NULL,
 encrypted_response TEXT NOT NULL,
 expires_at INTEGER NOT NULL,
 PRIMARY KEY(connection_id,request_hash)
);
-- +goose Down
DROP TABLE IF EXISTS connection_cache;
DROP TABLE IF EXISTS connection_usage;
DROP TABLE IF EXISTS artifact_connection_grants;
DROP TABLE IF EXISTS connections;
