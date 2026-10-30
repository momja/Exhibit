-- +goose Up
CREATE TABLE artifact_relay_grants (
    artifact_id TEXT NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
    viewer_id INTEGER NOT NULL,
    reads INTEGER NOT NULL DEFAULT 0 CHECK (reads IN (0,1)),
    writes INTEGER NOT NULL DEFAULT 0 CHECK (writes IN (0,1) AND writes <= reads),
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (artifact_id, viewer_id)
);
CREATE TABLE relay_activity (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    artifact_id TEXT NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
    viewer_id INTEGER NOT NULL,
    origin TEXT NOT NULL,
    method TEXT NOT NULL,
    status INTEGER NOT NULL,
    bytes INTEGER NOT NULL,
    duration_ms INTEGER NOT NULL,
    cached INTEGER NOT NULL DEFAULT 0,
    error_code TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX relay_activity_viewer ON relay_activity(artifact_id,viewer_id,id DESC);
CREATE TABLE relay_schedules (
    id TEXT PRIMARY KEY,
    artifact_id TEXT NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
    viewer_id INTEGER NOT NULL,
    url TEXT NOT NULL,
    state_key TEXT NOT NULL,
    interval_seconds INTEGER NOT NULL CHECK (interval_seconds >= 3600),
    next_run_at INTEGER NOT NULL,
    last_run_at INTEGER NOT NULL DEFAULT 0,
    last_status INTEGER NOT NULL DEFAULT 0,
    last_error TEXT NOT NULL DEFAULT '',
    enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0,1)),
    UNIQUE (artifact_id,viewer_id,state_key)
);
CREATE INDEX relay_schedules_due ON relay_schedules(enabled,next_run_at);
-- Viewer ids also support the single-user instance, which has no users row.
-- +goose StatementBegin
CREATE TRIGGER relay_account_delete AFTER DELETE ON users BEGIN
    DELETE FROM artifact_relay_grants WHERE viewer_id = OLD.id;
    DELETE FROM relay_activity WHERE viewer_id = OLD.id;
    DELETE FROM relay_schedules WHERE viewer_id = OLD.id;
END;
-- +goose StatementEnd
ALTER TABLE users ADD COLUMN connection_native_limit INTEGER CHECK (connection_native_limit >= 0);
ALTER TABLE users ADD COLUMN connection_oauth_limit INTEGER CHECK (connection_oauth_limit >= 0);
ALTER TABLE users ADD COLUMN connection_vendor_limit INTEGER CHECK (connection_vendor_limit >= 0);
ALTER TABLE users ADD COLUMN relay_schedule_limit INTEGER CHECK (relay_schedule_limit >= 0);

-- +goose Down
DROP TRIGGER relay_account_delete;
DROP TABLE relay_schedules;
DROP TABLE relay_activity;
DROP TABLE artifact_relay_grants;
ALTER TABLE users DROP COLUMN relay_schedule_limit;
ALTER TABLE users DROP COLUMN connection_vendor_limit;
ALTER TABLE users DROP COLUMN connection_oauth_limit;
ALTER TABLE users DROP COLUMN connection_native_limit;
