-- +goose Up
CREATE TABLE IF NOT EXISTS artifacts (
    id TEXT PRIMARY KEY,
    owner_id INTEGER NOT NULL DEFAULT 1,
    title TEXT NOT NULL DEFAULT '',
    source_blob_id TEXT NOT NULL,
    tier INTEGER NOT NULL DEFAULT 1,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now')),
    network_allowlist TEXT NOT NULL DEFAULT '[]'
);

CREATE TABLE IF NOT EXISTS collections (
    id TEXT PRIMARY KEY,
    owner_id INTEGER NOT NULL DEFAULT 1,
    name TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS artifact_collections (
    artifact_id TEXT NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
    collection_id TEXT NOT NULL REFERENCES collections(id) ON DELETE CASCADE,
    PRIMARY KEY (artifact_id, collection_id)
);

CREATE TABLE IF NOT EXISTS tags (
    id TEXT PRIMARY KEY,
    owner_id INTEGER NOT NULL DEFAULT 1,
    name TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS artifact_tags (
    artifact_id TEXT NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
    tag_id TEXT NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    PRIMARY KEY (artifact_id, tag_id)
);

CREATE TABLE IF NOT EXISTS artifact_state (
    artifact_id TEXT NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
    key TEXT NOT NULL,
    value TEXT NOT NULL,
    updated_at DATETIME NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (artifact_id, key)
);

CREATE TABLE IF NOT EXISTS shares (
    id TEXT PRIMARY KEY,
    artifact_id TEXT NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
    public INTEGER NOT NULL DEFAULT 1,
    expires_at DATETIME
);

-- FTS5 virtual table for full-text search
CREATE VIRTUAL TABLE IF NOT EXISTS artifacts_fts USING fts5(
    id UNINDEXED,
    title,
    content='artifacts',
    content_rowid='rowid'
);

-- Triggers to keep FTS in sync
-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS artifacts_fts_insert AFTER INSERT ON artifacts BEGIN
    INSERT INTO artifacts_fts(rowid, id, title) VALUES (new.rowid, new.id, new.title);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS artifacts_fts_delete AFTER DELETE ON artifacts BEGIN
    INSERT INTO artifacts_fts(artifacts_fts, rowid, id, title) VALUES ('delete', old.rowid, old.id, old.title);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS artifacts_fts_update AFTER UPDATE ON artifacts BEGIN
    INSERT INTO artifacts_fts(artifacts_fts, rowid, id, title) VALUES ('delete', old.rowid, old.id, old.title);
    INSERT INTO artifacts_fts(rowid, id, title) VALUES (new.rowid, new.id, new.title);
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER IF EXISTS artifacts_fts_update;
DROP TRIGGER IF EXISTS artifacts_fts_delete;
DROP TRIGGER IF EXISTS artifacts_fts_insert;
DROP VIRTUAL TABLE IF EXISTS artifacts_fts;
DROP TABLE IF EXISTS shares;
DROP TABLE IF EXISTS artifact_state;
DROP TABLE IF EXISTS artifact_tags;
DROP TABLE IF EXISTS tags;
DROP TABLE IF EXISTS artifact_collections;
DROP TABLE IF EXISTS collections;
DROP TABLE IF EXISTS artifacts;
