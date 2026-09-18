-- +goose Up
-- av-lrae: a share stops being only a capability URL and becomes a GRANT.
--
-- The distinction the epic (av-7k7b) settled on:
--
--   link   the unguessable id is the secret; the recipient opens /s/abc123
--   grant  the row says "user 7 may read artifact X"; they open /artifacts/X
--
-- Both are rows in this one table, told apart by recipient_id alone. NULL is
-- the anonymous link, where the id is still the whole authorization because
-- there is nobody at the door to check; set, it names an account on this
-- instance and the id buys nothing — a grantee who loses the bookmark is not
-- locked out of something they were given.
--
-- Version numbering: 8, 12 and 23 are Go migrations in migration_repair.go /
-- migration_origins.go with no file in this directory, and the highest .sql
-- here is 027. A migration must sit above *every* version that can already be
-- in a ledger, from either source: a number reused is skipped forever (four
-- outages, catalogued in migration_repair.go) and a number below the
-- high-water mark stops goose before it runs anything, so the instance does
-- not start. migration_order_test.go walks both rules.

-- The recipient. ON DELETE CASCADE is what retires a person's grants with
-- their account — the same job migration 014's trigger does for the state they
-- wrote, and a real foreign key is available here where it was not there:
-- artifact_state must hold rows for owner 1 on a static-token instance with an
-- empty `users` table, whereas a grant names a *named* account by definition.
-- SQLite requires an added REFERENCES column to default to NULL, which is
-- exactly what the anonymous link wants anyway.
ALTER TABLE shares ADD COLUMN recipient_id INTEGER REFERENCES users(id) ON DELETE CASCADE;

-- Whose rows a recipient writes when they use a shared artifact: their own
-- ('own') or the owner's shared board. It lives on the ARTIFACT, not on the
-- grant, because it is one question with one answer per artifact — per-grant
-- would admit Alice on her own list while Bob writes the owner's, a state
-- nobody wants and one that makes "whose board am I on" unexplainable on the
-- page. av-v991 owns the modes beyond the default and the UI that sets them;
-- this migration only establishes that the answer has a home.
ALTER TABLE artifacts ADD COLUMN share_state_mode TEXT NOT NULL DEFAULT 'own';

-- Collapse any artifact that already carries more than one share row, keeping
-- the earliest. Nothing before today stopped an owner minting a second link,
-- and every existing row becomes an anonymous one (recipient_id defaults to
-- NULL), so without this the partial index below fails and the instance does
-- not start — which is the one upgrade outcome worse than losing a duplicate
-- link. The earliest is kept because it is the one most likely to be in
-- somebody's hands already; the rest are revoked, which is precisely what the
-- "one anonymous link per artifact" decision means for a library that
-- accumulated several.
DELETE FROM shares WHERE rowid NOT IN (SELECT MIN(rowid) FROM shares GROUP BY artifact_id);

-- av-20xv: `public` was accepted, stored, and never read by ServeShare — an
-- access control the API advertised and did not have. It is dropped rather
-- than wired up because with recipient_id present it means exactly
-- `recipient_id IS NULL`: two columns encoding one fact, with nothing to stop
-- them disagreeing. Same reasoning av-8ipt applied to expires_at (015), and
-- doing it while every value is still uninterpreted keeps it a schema change
-- rather than a data question. DROP COLUMN's preconditions hold — no index,
-- view, trigger or constraint names it.
ALTER TABLE shares DROP COLUMN public;

-- One grant per (artifact, person), as a schema invariant rather than a
-- handler convention — the argument artifact_network_origins' primary key
-- already makes. Two grants of one artifact to one account would be two
-- answers to a question that has one.
CREATE UNIQUE INDEX shares_artifact_recipient ON shares(artifact_id, recipient_id);

-- And one anonymous link per artifact, which the index above does NOT deliver:
-- SQLite treats every NULL in a unique index as distinct from every other
-- NULL, so it constrains grants and says nothing at all about links. The
-- partial index is what makes the link singular, and it is what lets the UI be
-- a toggle — on mints the row and shows the URL, off deletes it — instead of a
-- button that accumulates links whose destinations nobody remembers. Partial
-- indexes have been in SQLite since 3.8; the pinned driver reports 3.53.
CREATE UNIQUE INDEX shares_one_anonymous_link ON shares(artifact_id) WHERE recipient_id IS NULL;

-- +goose Down
-- The honest reverse, and it is lossy in two ways worth naming rather than
-- hiding: the duplicate share rows deleted above do not come back (a dropped
-- row is gone, and nothing else recorded them), and every grant loses its
-- recipient — so a down migration turns each one into an anonymous link
-- readable by anyone holding its id. `public` returns at its original default,
-- which is the value every row carried in practice.
DROP INDEX IF EXISTS shares_one_anonymous_link;
DROP INDEX IF EXISTS shares_artifact_recipient;
ALTER TABLE shares ADD COLUMN public INTEGER NOT NULL DEFAULT 1;
ALTER TABLE artifacts DROP COLUMN share_state_mode;
ALTER TABLE shares DROP COLUMN recipient_id;
