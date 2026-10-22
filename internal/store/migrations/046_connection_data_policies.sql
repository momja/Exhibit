-- +goose Up
-- The credential grant can be revoked while its response remains in saved
-- state. Keep the data's destinations independently of credential lifetime.
-- No connection FK: deleting a credential must never release saved data.
CREATE TABLE artifact_connection_data_policies (
 artifact_id TEXT NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
 viewer_id INTEGER NOT NULL,
 connection_id TEXT NOT NULL,
 origins_json TEXT NOT NULL DEFAULT '[]',
 share_with_recipients INTEGER NOT NULL DEFAULT 0 CHECK(share_with_recipients IN (0,1)),
 PRIMARY KEY(artifact_id,viewer_id,connection_id)
);
CREATE INDEX connection_data_policies_viewer ON artifact_connection_data_policies(viewer_id,artifact_id);

INSERT INTO artifact_connection_data_policies(artifact_id,viewer_id,connection_id,origins_json,share_with_recipients)
 SELECT g.artifact_id,g.viewer_id,g.connection_id,
   (SELECT json_group_array(value) FROM (
     SELECT value FROM json_each(c.recipe_json,'$.origins')
     UNION SELECT value FROM json_each(g.permitted_egress_json) ORDER BY value
   )),g.share_with_recipients
 FROM artifact_connection_grants g JOIN connections c ON c.id=g.connection_id;

-- Existing recipient state may already contain a response received through an
-- owner's shared grant. Retain their custody too, including former recipients
-- whose rows would become readable again if access were granted later.
INSERT INTO artifact_connection_data_policies(artifact_id,viewer_id,connection_id,origins_json,share_with_recipients)
 SELECT DISTINCT p.artifact_id,s.user_id,p.connection_id,p.origins_json,0
 FROM artifact_connection_data_policies p JOIN artifacts a ON a.id=p.artifact_id
 JOIN artifact_state s ON s.artifact_id=p.artifact_id AND s.user_id!=a.owner_id
 WHERE p.viewer_id=a.owner_id AND p.share_with_recipients=1
 ON CONFLICT(artifact_id,viewer_id,connection_id) DO NOTHING;

-- +goose Down
DROP TABLE artifact_connection_data_policies;
