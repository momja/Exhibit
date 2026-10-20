-- +goose Up
-- Asset URLs are public and immutable. Keep their identity after every payload
-- reference is deleted, without retaining the payload or charging its bytes.
CREATE TABLE asset_identities (
    id TEXT PRIMARY KEY,
    artifact_id TEXT NOT NULL,
    blob_id TEXT NOT NULL,
    content_type TEXT NOT NULL
);

INSERT OR IGNORE INTO asset_identities (id, artifact_id, blob_id, content_type)
SELECT id, artifact_id, blob_id, content_type FROM artifact_assets
UNION ALL
SELECT id, artifact_id, blob_id, content_type FROM artifact_version_assets;

-- +goose StatementBegin
CREATE TRIGGER remember_current_asset_identity AFTER INSERT ON artifact_assets
BEGIN
    INSERT OR IGNORE INTO asset_identities (id, artifact_id, blob_id, content_type)
    VALUES (NEW.id, NEW.artifact_id, NEW.blob_id, NEW.content_type);
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER remember_historical_asset_identity AFTER INSERT ON artifact_version_assets
BEGIN
    INSERT OR IGNORE INTO asset_identities (id, artifact_id, blob_id, content_type)
    VALUES (NEW.id, NEW.artifact_id, NEW.blob_id, NEW.content_type);
END;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER remember_historical_asset_identity;
DROP TRIGGER remember_current_asset_identity;
DROP TABLE asset_identities;
