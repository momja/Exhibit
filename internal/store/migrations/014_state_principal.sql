-- +goose Up
-- av-q0ub: artifact_state records which *tool* data belongs to, but not whose
-- data it is. Re-key it by (artifact_id, user_id, key).
--
-- user_id is the VIEWER, deliberately not named owner_id. On a shared artifact
-- the viewer and the owner are different people, and the two questions the
-- state layer asks are different questions:
--
--   owner_id  — may this caller reach this artifact at all?  (authorization)
--   user_id   — whose rows are these?                        (selection)
--
-- A column named owner_id here would read as the first and be used as the
-- second at exactly the call sites where the distinction decides who sees what.
--
-- Version numbering: 8 and 12 are occupied by Go migrations in
-- migration_repair.go with no file in this directory, and 013 is the users /
-- sessions table. Reusing a version silently skips the migration forever on any
-- database that took the first one — read migration_repair.go's header before
-- adding another.

-- SQLite cannot ALTER a primary key, so rebuild and copy.
CREATE TABLE artifact_state_new (
    artifact_id TEXT NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
    user_id INTEGER NOT NULL,
    key TEXT NOT NULL,
    value TEXT NOT NULL,
    updated_at DATETIME NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (artifact_id, user_id, key)
);

-- Backfill: every pre-existing row was written by the artifact's own owner,
-- because that is the only principal that could reach it. The JOIN is an inner
-- join on purpose — a state row whose artifact is gone is already unreachable
-- (the ON DELETE CASCADE above has been in force since 001), and the new table
-- could not hold it anyway.
INSERT INTO artifact_state_new (artifact_id, user_id, key, value, updated_at)
SELECT s.artifact_id, a.owner_id, s.key, s.value, s.updated_at
  FROM artifact_state s
  JOIN artifacts a ON a.id = s.artifact_id;

DROP TABLE artifact_state;
ALTER TABLE artifact_state_new RENAME TO artifact_state;

-- Serves the per-user delete below, and any future "everything this viewer has
-- accumulated" query (quotas, export, account deletion).
CREATE INDEX IF NOT EXISTS idx_artifact_state_user ON artifact_state(user_id);

-- The second cascade (AC#4). State already dies with its artifact; a
-- user-scoped table needs it to die with its viewer too.
--
-- This is a trigger and not `REFERENCES users(id) ON DELETE CASCADE`, for one
-- decisive reason: an owner id does not imply a users row. Single-user
-- instances run on the static token with owner_id 1 and an empty `users` table
-- — 013's own note says the first identity to log in *becomes* user 1 and
-- adopts that library. With PRAGMA foreign_keys=ON a real FK would therefore
-- reject every state write on exactly the deployment that is most common
-- today. No other owner-bearing column in this schema (artifacts.owner_id,
-- tags.owner_id, collections.owner_id) carries such an FK either; the trigger
-- buys the cascade without the referential precondition.
-- +goose StatementBegin
CREATE TRIGGER IF NOT EXISTS artifact_state_user_delete
AFTER DELETE ON users
BEGIN
    DELETE FROM artifact_state WHERE user_id = OLD.id;
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER IF EXISTS artifact_state_user_delete;
DROP INDEX IF EXISTS idx_artifact_state_user;

CREATE TABLE artifact_state_old (
    artifact_id TEXT NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
    key TEXT NOT NULL,
    value TEXT NOT NULL,
    updated_at DATETIME NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (artifact_id, key)
);

-- Only the artifact owner's rows are representable without a principal column,
-- so a down migration necessarily drops any other viewer's state. That is the
-- honest reverse of this change, not an oversight.
INSERT INTO artifact_state_old (artifact_id, key, value, updated_at)
SELECT s.artifact_id, s.key, s.value, s.updated_at
  FROM artifact_state s
  JOIN artifacts a ON a.id = s.artifact_id AND a.owner_id = s.user_id;

DROP TABLE artifact_state;
ALTER TABLE artifact_state_old RENAME TO artifact_state;
