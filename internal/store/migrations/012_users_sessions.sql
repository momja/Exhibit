-- +goose Up
-- Identity and sessions (av-30rj).
--
-- The identity provider is a login-time concern only: it is exchanged exactly
-- once, at /auth/callback, for a row in `sessions`. Everything downstream —
-- the cookie, the lookup, owner_id — is ours and identical whichever provider
-- issued the identity. That is what keeps the provider swap contained to one
-- constructor.
--
-- `email` is stored beside `external_id` on purpose. A provider subject is
-- provider-specific, so an instance that changes IdP has no way to recognize
-- a returning user without a second, portable key. Recording it costs one
-- column now and is painful to retrofit once identities exist.
CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    external_id TEXT NOT NULL UNIQUE,
    email TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);

-- The session id in the cookie is opaque random bytes looked up here on every
-- request, rather than a signed token carrying its own claims. That is the
-- whole point: a row can be deleted, so logout revokes immediately instead of
-- at some TTL the server cannot influence.
CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    expires_at DATETIME NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id);

-- Note on owner ids: every pre-existing row in this database is owned by
-- owner_id 1, the single-user default. `users.id` is AUTOINCREMENT from 1, so
-- on an instance upgraded from single-user the first identity to log in becomes
-- user 1 and adopts that existing library. That is the intended upgrade path
-- for an operator enabling an IdP on their own instance, and it is why
-- deployment.md tells operators to complete the first login themselves before
-- opening the instance to anyone else.

-- +goose Down
DROP INDEX IF EXISTS idx_sessions_user;
DROP TABLE IF EXISTS sessions;
DROP INDEX IF EXISTS idx_users_email;
DROP TABLE IF EXISTS users;
