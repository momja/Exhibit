-- +goose Up
-- A Clerk session may outlive the local library it authenticated. Retain only
-- the identity and erasure cutoff, independently of users, so those sessions
-- cannot recreate the account. A fresh provider session may start a new library.
CREATE TABLE hosted_account_deletions (
    external_id TEXT PRIMARY KEY,
    deleted_at_ms INTEGER NOT NULL
);

-- +goose Down
DROP TABLE hosted_account_deletions;
