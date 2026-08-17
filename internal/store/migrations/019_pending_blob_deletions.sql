-- +goose Up
-- The blob deletion queue (av-8gyd). Deleting bytes spans two stores — rows
-- here, files in the blob store — and cannot be made atomic, so what this
-- table makes durable is the *intent*: the transaction that removes the last
-- row naming a blob inserts its id here, in that same transaction. After the
-- commit, deleting the file and then this row is idempotent work any later
-- process can finish, which is why a crash between the two can leak nothing.
--
-- Version 19 and not 13: goose records applied migrations by number alone, so
-- a number once applied is "already applied" forever. Versions 8 and 12 are
-- consumed by the Go repairs in migration_repair.go (they heal the collisions
-- at 5 and 11), and 18 is the highest .sql file — reusing any of them would
-- silently skip this table on every deployed database (av-9pm8).
--
-- blob_id is the primary key rather than a serial: the queue holds ids, not
-- attempts, and enqueuing one twice must mean the same thing as enqueuing it
-- once. created_at is diagnostic only — nothing reads it to decide anything,
-- because there is no scheduler here to age rows out.
CREATE TABLE IF NOT EXISTS pending_blob_deletions (
    blob_id    TEXT PRIMARY KEY,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- +goose Down
DROP TABLE IF EXISTS pending_blob_deletions;
