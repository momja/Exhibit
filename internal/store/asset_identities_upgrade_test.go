package store

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAssetIdentityUpgradeRetainsDeletedURLBindings(t *testing.T) {
	registerOriginNormalizationMigration()
	path := newMigratedTo(t, 33)
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	legacy := &SQLiteStore{db: db}
	ctx := t.Context()
	require.NoError(t, legacy.PutArtifact(ctx, &Artifact{ID: "artifact", OwnerID: 1, Title: "Legacy", SourceBlobID: "body", Tier: Tier1}))
	old := asset("historical", "https://example.test/old", "old-blob")
	_, err = legacy.ReplaceArtifactAssets(ctx, 1, "artifact", "old", []ArtifactAsset{old})
	require.NoError(t, err)
	_, err = legacy.CommitVersion(ctx, 1, "artifact", VersionChange{BodyBlobID: strPtr("new-body")})
	require.NoError(t, err)
	current := asset("current", "https://example.test/current", "current-blob")
	_, err = legacy.ReplaceArtifactAssets(ctx, 1, "artifact", "current", []ArtifactAsset{current})
	require.NoError(t, err)
	require.NoError(t, db.Close())

	s, err := OpenSQLite(path)
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	var remembered int
	require.NoError(t, s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM asset_identities").Scan(&remembered))
	assert.Equal(t, 2, remembered, "migration backfills both current and historical URLs")
	_, err = s.DeleteArtifact(ctx, 1, "artifact")
	require.NoError(t, err)
	require.NoError(t, s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM asset_identities").Scan(&remembered))
	assert.Equal(t, 2, remembered, "deleting an artifact cannot release its public URL identities")
	require.NoError(t, s.PutArtifact(ctx, &Artifact{ID: "artifact", OwnerID: 1, Title: "Recreated", SourceBlobID: "body2", Tier: Tier1}))
	for _, original := range []ArtifactAsset{old, current} {
		changed := original
		changed.BlobID = "different-blob"
		_, err = s.ImportArtifactAssets(ctx, 1, "artifact", "changed", []ArtifactAsset{changed}, true)
		require.ErrorIs(t, err, ErrImportAssetConflict)
		changed = original
		changed.ContentType = "text/html"
		_, err = s.ImportArtifactAssets(ctx, 1, "artifact", "changed", []ArtifactAsset{changed}, true)
		require.ErrorIs(t, err, ErrImportAssetConflict)
	}
}
