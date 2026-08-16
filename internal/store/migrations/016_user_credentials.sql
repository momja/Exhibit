-- +goose Up
-- Local credentials move out of the environment and onto the users row
-- (av-rzvf), so an instance can have more than one local user.
--
-- Version numbering: 8 and 12 are occupied by Go migrations in
-- migration_repair.go that have no file in this directory, and the highest
-- file here is 014. Reusing a version silently skips the migration forever on
-- any database that took the first one — read migration_repair.go's header
-- before adding another.

-- password_hash is NULLABLE, and that is the load-bearing part of this
-- migration. An OIDC identity has no password and must stay a first-class row
-- in this same table and this same owner_id space; a local account is the same
-- row with this column filled in. The two kinds of user therefore differ by
-- which columns are populated, not by living in separate tables — which is
-- what stops "the same person has an SSO login and a local login" from
-- becoming an account-linking problem in the schema rather than a policy
-- decision we can take later.
--
-- NULL is "this account has no password", which is not the same as '' — an
-- empty string would be a hash that no bcrypt comparison can ever match, i.e.
-- a password-disabled account. Both mean "cannot log in locally" today, but
-- only NULL says why.
ALTER TABLE users ADD COLUMN password_hash TEXT;

-- The role marker is a boolean rather than a `role TEXT`, because there are
-- exactly two roles — the person who administers the instance and everyone
-- else — and a text column would invite a taxonomy nobody has designed. A
-- third role, if one is ever wanted, is a migration; an unconstrained string
-- is a permanent invitation to invent values at the call site.
ALTER TABLE users ADD COLUMN is_admin INTEGER NOT NULL DEFAULT 0;

-- Before this migration there was exactly one local credential per instance,
-- so its users row could be keyed on the constant 'local' and the username was
-- a label on it. With more than one local user the name *is* the key, so local
-- rows are re-keyed to 'local:<email>'.
--
-- This reuses external_id's existing UNIQUE constraint rather than adding one
-- to email: uniqueness of local logins becomes a schema invariant for free,
-- and it does not constrain OIDC rows, whose email is whatever the provider
-- last reported and is not ours to make unique.
--
-- Re-keying the existing row (rather than leaving 'local' beside the new
-- scheme) is what lets an operator upgrade in place: their configured
-- LOGIN_USERNAME still resolves to the library it already owns.
UPDATE users SET external_id = 'local:' || lower(trim(email))
 WHERE external_id = 'local' AND trim(email) <> '';

-- The first user on an instance is its admin. On a database that already has
-- users, that is the row that adopted owner 1's library at the first login —
-- deployment.md §3.4 has always told operators to complete that login
-- themselves for exactly this reason, so this records an existing fact rather
-- than granting anything new. New instances get the same rule applied at
-- insert time, in sqlite_users.go.
UPDATE users SET is_admin = 1 WHERE id = (SELECT min(id) FROM users);

-- +goose Down
-- Only one row can become 'local' without violating external_id's UNIQUE
-- constraint. With more than one local account (possible once this migration
-- has run and an operator added a second), picking a survivor at random would
-- silently orphan the rest; restoring the oldest — the row this migration's Up
-- direction re-keyed in the first place — is the least surprising of the
-- available choices.
UPDATE users SET external_id = 'local'
 WHERE id = (SELECT min(id) FROM users WHERE external_id LIKE 'local:%');
ALTER TABLE users DROP COLUMN is_admin;
ALTER TABLE users DROP COLUMN password_hash;
