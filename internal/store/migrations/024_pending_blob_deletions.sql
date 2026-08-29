-- +goose Up
-- The blob deletion queue (av-8gyd). Deleting bytes spans two stores — rows
-- here, files in the blob store — and cannot be made atomic, so what this
-- table makes durable is the *intent*: the transaction that removes the last
-- row naming a blob inserts its id here, in that same transaction. After the
-- commit, deleting the file and then this row is idempotent work any later
-- process can finish, which is why a crash between the two can leak nothing.
--
-- Version 24, and the number is the interesting part. goose records applied
-- migrations by number alone, so a number once applied is "already applied"
-- forever — but the failure this file actually met is the *other* one: goose
-- refuses to start at all when it finds an unapplied migration numbered below
-- the ledger's high-water mark ("found N missing migrations before current
-- version"). This table was written as 19 while 18 was the highest file, and
-- by the time the branch landed main had shipped 021, 022 and a Go migration
-- at 23, so every instance that took main would have refused to boot.
--
-- Reserving a low number for work in flight does not survive that: the
-- reservation holds only if the branch merges before anything above it
-- deploys. New migrations therefore take the next number above *everything*
-- already in the ledger — .sql files and the Go migrations in
-- migration_repair.go / migration_origins.go alike, which is what puts 8, 12
-- and 23 out of reach. Read migration_repair.go's header before adding
-- another, and never renumber one that has run anywhere real.
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
