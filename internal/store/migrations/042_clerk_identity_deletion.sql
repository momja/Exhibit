-- +goose Up
-- Legacy library-only deletions remain session cutoffs. New account deletions
-- permanently fence the identity and retain provider work until it succeeds.
ALTER TABLE hosted_account_deletions ADD COLUMN delete_identity INTEGER NOT NULL DEFAULT 0;
ALTER TABLE hosted_account_deletions ADD COLUMN identity_deleted INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE hosted_account_deletions DROP COLUMN identity_deleted;
ALTER TABLE hosted_account_deletions DROP COLUMN delete_identity;
