-- +goose Up
-- Existing managed credentials belong to the original engine. External IDs
-- stay in their encrypted legacy credential until the service reads and pins
-- them; SQL migrations cannot and must not decrypt credential payloads.
ALTER TABLE connections ADD COLUMN engine TEXT NOT NULL DEFAULT '';
ALTER TABLE connections ADD COLUMN external_ref TEXT NOT NULL DEFAULT '';
UPDATE connections SET engine='nango' WHERE kind='vendor';
UPDATE connections SET provider=CASE WHEN json_valid(recipe_json)
 THEN COALESCE(NULLIF(json_extract(recipe_json,'$.vendor_provider'),''),provider)
 ELSE provider END WHERE kind='vendor';
CREATE INDEX connections_vendor_engine ON connections(engine,provider) WHERE kind='vendor';

-- Aggregate operator accounting contains no account, connection, artifact,
-- request URL, body or credential identity. Historical spend survives deletion.
CREATE TABLE vendor_usage (
 engine TEXT NOT NULL CHECK(engine IN ('nango','composio')),
 provider TEXT NOT NULL,
 day TEXT NOT NULL,
 management_calls INTEGER NOT NULL DEFAULT 0 CHECK(management_calls>=0),
 forward_calls INTEGER NOT NULL DEFAULT 0 CHECK(forward_calls>=0),
 inject_calls INTEGER NOT NULL DEFAULT 0 CHECK(inject_calls>=0),
 PRIMARY KEY(engine,provider,day)
);

-- +goose Down
DROP TABLE vendor_usage;
DROP INDEX connections_vendor_engine;
ALTER TABLE connections DROP COLUMN external_ref;
ALTER TABLE connections DROP COLUMN engine;
