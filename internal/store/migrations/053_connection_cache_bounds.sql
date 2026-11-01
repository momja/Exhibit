-- +goose Up
-- Account for encoded ciphertext once, rather than loading every response
-- while holding the writer lock for each cache insertion.
ALTER TABLE connection_cache ADD COLUMN encrypted_bytes INTEGER NOT NULL DEFAULT 0 CHECK(encrypted_bytes>=0);
UPDATE connection_cache SET encrypted_bytes=length(CAST(encrypted_response AS BLOB));
CREATE INDEX connection_cache_expiry ON connection_cache(expires_at DESC,connection_id,request_hash,encrypted_bytes);
CREATE INDEX connection_cache_retention ON connection_cache(connection_id,expires_at DESC,request_hash,encrypted_bytes);

-- +goose Down
DROP INDEX IF EXISTS connection_cache_retention;
DROP INDEX IF EXISTS connection_cache_expiry;
ALTER TABLE connection_cache DROP COLUMN encrypted_bytes;
