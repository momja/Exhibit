-- +goose Up
-- Exh-tjib: find one owner's library without scanning every artifact.
CREATE INDEX artifacts_owner ON artifacts(owner_id);

-- +goose Down
DROP INDEX artifacts_owner;
