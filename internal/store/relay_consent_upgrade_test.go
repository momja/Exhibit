package store

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Exh-cxzb: one relay consent replaces the reads/writes pair, and connection
// recipes lose allow_writes. A reads-only grant was given under copy that
// promised GET and HEAD only, so the upgrade asks again rather than widening it.
func TestRelayConsentUpgradeKeepsOnlyFullGrantsAndDropsAllowWrites(t *testing.T) {
	registerOriginNormalizationMigration()
	path := newMigratedTo(t, 48)
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	legacy := &SQLiteStore{db: db}
	ctx := t.Context()
	for _, id := range []string{"full", "reads-only", "none"} {
		require.NoError(t, legacy.PutArtifact(ctx, &Artifact{ID: id, OwnerID: 1, Title: id, SourceBlobID: "body-" + id, Tier: Tier1}))
	}
	_, err = db.ExecContext(ctx, `INSERT INTO artifact_relay_grants(artifact_id,viewer_id,reads,writes) VALUES ('full',1,1,1),('reads-only',1,1,0),('none',1,0,0)`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO connections(id,owner_id,name,provider,kind,recipe_json,encrypted_secret) VALUES
 ('native',1,'API','bearer','native','{"origins":["https://api.example.test"],"allow_writes":false,"daily_limit":3}','sealed'),
 ('vendor',1,'Notes','notes','vendor','{"origins":["https://api.example.test"],"allow_writes":true,"vendor_spec":{"name":"Notes","allow_writes":true}}','sealed')`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	s, err := OpenSQLite(path)
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	for id, want := range map[string]bool{"full": true, "reads-only": false, "none": false} {
		g, err := s.GetRelayGrant(ctx, 1, id)
		require.NoError(t, err)
		assert.Equal(t, want, g.Allowed, id)
	}
	var remaining int
	require.NoError(t, s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM artifact_relay_grants`).Scan(&remaining))
	assert.Equal(t, 1, remaining, "cleared grants are removed, not kept as refusals")

	recipes := map[string]string{}
	rows, err := s.db.QueryContext(ctx, `SELECT id, recipe_json FROM connections`)
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		var id, recipe string
		require.NoError(t, rows.Scan(&id, &recipe))
		recipes[id] = recipe
	}
	require.NoError(t, rows.Err())
	assert.JSONEq(t, `{"origins":["https://api.example.test"],"daily_limit":3}`, recipes["native"])
	assert.JSONEq(t, `{"origins":["https://api.example.test"],"vendor_spec":{"name":"Notes"}}`, recipes["vendor"])
}
