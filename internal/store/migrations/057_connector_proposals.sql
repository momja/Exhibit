-- +goose Up
-- Agent proposals contain only public setup metadata. Credentials live solely
-- in connections.encrypted_secret after a private, owner-approved submission.
CREATE TABLE connector_proposals (
 id TEXT PRIMARY KEY,
 owner_id INTEGER NOT NULL,
 artifact_id TEXT NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
 name TEXT NOT NULL,
 origin TEXT NOT NULL,
 header TEXT NOT NULL,
 auth TEXT NOT NULL CHECK(auth IN ('api-key','bearer')),
 reason TEXT NOT NULL,
 state TEXT NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','approved','denied')),
 connection_id TEXT REFERENCES connections(id) ON DELETE SET NULL,
 created_at DATETIME NOT NULL DEFAULT (datetime('now')),
 UNIQUE(owner_id,artifact_id,origin,auth,header)
);

-- +goose Down
DROP TABLE connector_proposals;
