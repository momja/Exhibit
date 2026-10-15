package store

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Main already shipped 031 for geolocation. Version history must run after
// that ledger entry, backfill existing artifacts, and keep their approvals.
func TestVersionHistoryUpgradesAnInstanceWithGeolocation(t *testing.T) {
	registerOriginNormalizationMigration()
	path := newMigratedTo(t, 31)
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO artifacts (id, owner_id, title, source_blob_id, tier, geolocation_approved)
		VALUES ('existing', 1, 'Existing artifact', 'existing-body', 1, 1)`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	upgraded, err := OpenSQLite(path)
	require.NoError(t, err)
	defer upgraded.Close()

	var approved bool
	require.NoError(t, upgraded.db.QueryRowContext(context.Background(),
		`SELECT geolocation_approved FROM artifacts WHERE id = 'existing'`).Scan(&approved))
	assert.True(t, approved, "a shipped geolocation approval survives the upgrade")
	var seq int64
	var body, origin string
	require.NoError(t, upgraded.db.QueryRowContext(context.Background(),
		`SELECT seq, body_blob_id, origin FROM artifact_versions WHERE artifact_id = 'existing'`).Scan(&seq, &body, &origin))
	assert.EqualValues(t, 1, seq)
	assert.Equal(t, "existing-body", body)
	assert.Equal(t, "initial", origin)
	assert.Equal(t, headVersion(t), currentVersion(t, upgraded))
}
