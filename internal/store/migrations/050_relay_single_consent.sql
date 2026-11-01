-- +goose Up
-- Exh-cxzb: relay consent and connections no longer split "read" from "write"
-- by HTTP method. A method says nothing reliable about what a request does
-- (a GET can change things, a GraphQL query is a POST), and only the
-- provider's credential scope decides what a connection may do.
--
-- A grant that already covered every method keeps working. A reads-only grant
-- was agreed to under copy that promised GET and HEAD only, so it is cleared
-- rather than silently widened; the viewer is asked again.
DELETE FROM artifact_relay_grants WHERE writes = 0 OR reads = 0;
ALTER TABLE artifact_relay_grants DROP COLUMN writes;
ALTER TABLE artifact_relay_grants RENAME COLUMN reads TO allowed;
UPDATE connections SET recipe_json = json_remove(recipe_json, '$.allow_writes', '$.vendor_spec.allow_writes')
 WHERE json_valid(recipe_json);

-- +goose Down
ALTER TABLE artifact_relay_grants RENAME COLUMN allowed TO reads;
ALTER TABLE artifact_relay_grants ADD COLUMN writes INTEGER NOT NULL DEFAULT 0 CHECK (writes IN (0,1));
UPDATE artifact_relay_grants SET writes = reads;
