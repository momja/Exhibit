-- +goose Up
-- Remote authentication may finish before the local connection can be saved.
-- Keep its pinned identity until it is adopted locally or revoked remotely.
-- No user cascade: even unexpected account deletion must not discard cleanup.
CREATE TABLE pending_vendor_connections (
 id TEXT PRIMARY KEY,
 owner_id INTEGER NOT NULL,
 request_json TEXT NOT NULL,
 expires_at INTEGER NOT NULL,
 cleanup_at INTEGER NOT NULL
);
CREATE INDEX pending_vendor_connections_expiry ON pending_vendor_connections(cleanup_at);
CREATE INDEX pending_vendor_connections_owner ON pending_vendor_connections(owner_id);

-- +goose Down
DROP TABLE pending_vendor_connections;
