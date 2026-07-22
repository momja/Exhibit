-- +goose Up
-- PRD §8.2 / architecture §3.3 promise full-text search over artifact source
-- and tag text, not just title. artifacts.source_text and .tags_text are
-- denormalized search shadows of data that lives elsewhere (the blob store
-- and the tags/artifact_tags join, respectively) — FTS5 needs the text
-- inside SQLite to index it. source_text holds the body's *visible* text
-- (markup/script/style stripped by store.ExtractSearchText), populated by the
-- API whenever it writes the blob body (see internal/api/artifacts.go);
-- existing rows are backfilled from the blob store once at startup
-- (SQLiteStore.BackfillSourceText, since blob content isn't reachable from
-- SQL). tags_text is kept in sync entirely by triggers below and is
-- backfilled inline, since tag data is already in SQLite.
ALTER TABLE artifacts ADD COLUMN source_text TEXT NOT NULL DEFAULT '';
ALTER TABLE artifacts ADD COLUMN tags_text TEXT NOT NULL DEFAULT '';

UPDATE artifacts SET tags_text = (
    SELECT COALESCE(GROUP_CONCAT(t.name, ' '), '')
    FROM artifact_tags at JOIN tags t ON t.id = at.tag_id
    WHERE at.artifact_id = artifacts.id
);

-- FTS5 virtual tables can't be ALTERed to add columns; drop and recreate.
DROP TRIGGER IF EXISTS artifacts_fts_insert;
DROP TRIGGER IF EXISTS artifacts_fts_delete;
DROP TRIGGER IF EXISTS artifacts_fts_update;
DROP TABLE IF EXISTS artifacts_fts;

CREATE VIRTUAL TABLE artifacts_fts USING fts5(
    id UNINDEXED,
    title,
    source_text,
    tags_text,
    content='artifacts',
    content_rowid='rowid'
);

-- fts5 column names now match the content table's column names exactly, so
-- 'rebuild' can populate the index directly from the artifacts table.
INSERT INTO artifacts_fts(artifacts_fts) VALUES ('rebuild');

-- +goose StatementBegin
CREATE TRIGGER artifacts_fts_insert AFTER INSERT ON artifacts BEGIN
    INSERT INTO artifacts_fts(rowid, id, title, source_text, tags_text)
    VALUES (new.rowid, new.id, new.title, new.source_text, new.tags_text);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER artifacts_fts_delete AFTER DELETE ON artifacts BEGIN
    INSERT INTO artifacts_fts(artifacts_fts, rowid, id, title, source_text, tags_text)
    VALUES ('delete', old.rowid, old.id, old.title, old.source_text, old.tags_text);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER artifacts_fts_update AFTER UPDATE ON artifacts BEGIN
    INSERT INTO artifacts_fts(artifacts_fts, rowid, id, title, source_text, tags_text)
    VALUES ('delete', old.rowid, old.id, old.title, old.source_text, old.tags_text);
    INSERT INTO artifacts_fts(rowid, id, title, source_text, tags_text)
    VALUES (new.rowid, new.id, new.title, new.source_text, new.tags_text);
END;
-- +goose StatementEnd

-- artifacts.tags_text is a denormalized rollup of this artifact's tag names,
-- kept in sync whenever tag membership changes. Its own UPDATE on artifacts
-- then rides the artifacts_fts_update trigger above, so the fts index never
-- needs its own tag-membership triggers.
-- +goose StatementBegin
CREATE TRIGGER artifact_tags_text_sync_insert AFTER INSERT ON artifact_tags BEGIN
    UPDATE artifacts SET tags_text = (
        SELECT COALESCE(GROUP_CONCAT(t.name, ' '), '')
        FROM artifact_tags at JOIN tags t ON t.id = at.tag_id
        WHERE at.artifact_id = new.artifact_id
    ) WHERE id = new.artifact_id;
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER artifact_tags_text_sync_delete AFTER DELETE ON artifact_tags BEGIN
    UPDATE artifacts SET tags_text = (
        SELECT COALESCE(GROUP_CONCAT(t.name, ' '), '')
        FROM artifact_tags at JOIN tags t ON t.id = at.tag_id
        WHERE at.artifact_id = old.artifact_id
    ) WHERE id = old.artifact_id;
END;
-- +goose StatementEnd

-- Renaming a tag doesn't touch artifact_tags rows, so the two triggers above
-- won't fire for it; re-roll tags_text for every artifact carrying this tag.
-- +goose StatementBegin
CREATE TRIGGER tags_text_sync_rename AFTER UPDATE OF name ON tags
WHEN old.name IS NOT new.name BEGIN
    UPDATE artifacts SET tags_text = (
        SELECT COALESCE(GROUP_CONCAT(t.name, ' '), '')
        FROM artifact_tags at JOIN tags t ON t.id = at.tag_id
        WHERE at.artifact_id = artifacts.id
    ) WHERE id IN (SELECT artifact_id FROM artifact_tags WHERE tag_id = new.id);
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER IF EXISTS tags_text_sync_rename;
DROP TRIGGER IF EXISTS artifact_tags_text_sync_delete;
DROP TRIGGER IF EXISTS artifact_tags_text_sync_insert;
DROP TRIGGER IF EXISTS artifacts_fts_update;
DROP TRIGGER IF EXISTS artifacts_fts_delete;
DROP TRIGGER IF EXISTS artifacts_fts_insert;
DROP TABLE IF EXISTS artifacts_fts;

CREATE VIRTUAL TABLE artifacts_fts USING fts5(
    id UNINDEXED,
    title,
    content='artifacts',
    content_rowid='rowid'
);
INSERT INTO artifacts_fts(artifacts_fts) VALUES ('rebuild');

-- +goose StatementBegin
CREATE TRIGGER artifacts_fts_insert AFTER INSERT ON artifacts BEGIN
    INSERT INTO artifacts_fts(rowid, id, title) VALUES (new.rowid, new.id, new.title);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER artifacts_fts_delete AFTER DELETE ON artifacts BEGIN
    INSERT INTO artifacts_fts(artifacts_fts, rowid, id, title) VALUES ('delete', old.rowid, old.id, old.title);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER artifacts_fts_update AFTER UPDATE ON artifacts BEGIN
    INSERT INTO artifacts_fts(artifacts_fts, rowid, id, title) VALUES ('delete', old.rowid, old.id, old.title);
    INSERT INTO artifacts_fts(rowid, id, title) VALUES (new.rowid, new.id, new.title);
END;
-- +goose StatementEnd

-- SQLite cannot easily DROP a column without recreating the table; leave as a no-op.
