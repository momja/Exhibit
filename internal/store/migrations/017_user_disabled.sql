-- +goose Up
-- Disabling an account (av-utap): the admin's answer to "this person should
-- not be able to get in any more", without deleting the row and therefore
-- without orphaning the library that row owns.
--
-- Version numbering: 8 and 12 are occupied by Go migrations in
-- migration_repair.go that have no file in this directory, and the highest
-- file here is 016. Reusing a version silently skips the migration forever on
-- any database that took the first one — read migration_repair.go's header
-- before adding another.

-- DECISION — disabled is a *column*, not the removal of `password_hash`.
--
-- Clearing the hash was the tempting alternative: it needs no schema change,
-- and SetLocalPassword already accepts an empty hash for exactly that effect.
-- It does not generalise. An identity a provider issued has no hash to remove,
-- so it would leave the one kind of account this instance did not issue
-- un-disable-able — the OIDC half of the same users table, in the same
-- owner_id space. A column says "this account may not sign in" about *every*
-- row, whatever populated it, and it says it without destroying the credential
-- an admin may want to restore.
--
-- It is a nullable timestamp rather than a boolean because "when" is free here
-- and answers the question an operator actually asks of a disabled account.
-- NULL is enabled — the default every existing row takes, so this migration
-- changes nothing about an instance that never disables anybody.
ALTER TABLE users ADD COLUMN disabled_at TEXT;

-- +goose Down
ALTER TABLE users DROP COLUMN disabled_at;
