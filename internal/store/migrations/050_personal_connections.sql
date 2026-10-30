-- +goose Up
-- Sharing a tool or saved results never authorizes another person to use an
-- account. Retain the owner's grants and every saved result while withdrawing
-- the old cross-account delegation. Historical provenance remains data only.
UPDATE artifact_connection_grants SET share_with_recipients=0;
UPDATE artifact_connection_data_policies SET share_with_recipients=0;

-- A recipient's personal account may already have written results into the
-- owner's shared state. Preserve its provenance after sharing mode changes.
INSERT INTO artifact_connection_data_policies(artifact_id,viewer_id,connection_id,origins_json,share_with_recipients)
 SELECT p.artifact_id,a.owner_id,p.connection_id,p.origins_json,0
 FROM artifact_connection_data_policies p JOIN artifacts a ON a.id=p.artifact_id
 WHERE a.share_state_mode='shared' AND p.viewer_id!=a.owner_id
 ON CONFLICT(artifact_id,viewer_id,connection_id) DO NOTHING;

-- +goose Down
-- Delegation cannot be restored without a new decision by the account owner.
SELECT 1;
