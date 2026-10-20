package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImportStateRollsBackFailedReplacement(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	require.NoError(t, s.PutArtifact(ctx, &Artifact{ID: "import", OwnerID: 1, Title: "Import", SourceBlobID: "body", Tier: Tier1}))
	require.NoError(t, s.SetState(ctx, 1, "import", 1, "old", "keep"))
	_, err := s.db.ExecContext(ctx, `CREATE TRIGGER reject_import BEFORE INSERT ON artifact_state WHEN NEW.key='fail' BEGIN SELECT RAISE(ABORT, 'injected failure'); END`)
	require.NoError(t, err)
	require.Error(t, s.ImportState(ctx, 1, "import", map[string]string{"new": "value", "fail": "value"}, true))
	got, err := s.GetState(ctx, 1, "import", 1)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"old": "keep"}, got)
}

func TestImportAssetsChecksConfirmationAndRollsBackFailedInserts(t *testing.T) {
	fx := newQueueFixture(t)
	seedAssetArtifact(t, fx, "a1")
	ctx := t.Context()
	original := []ArtifactAsset{{ID: "old", SourceURL: "https://example.test/old", BlobID: "old-blob", ContentType: "image/png", SizeBytes: 5}}
	_, err := fx.store.ReplaceArtifactAssets(ctx, 1, "a1", "original", original)
	require.NoError(t, err)
	replacement := []ArtifactAsset{
		{ID: "first", SourceURL: "https://example.test/first", BlobID: "first-blob", ContentType: "image/png", SizeBytes: 5},
		{ID: "new", SourceURL: "https://example.test/new", BlobID: "new-blob", ContentType: "image/png", SizeBytes: 6},
	}
	_, err = fx.store.ImportArtifactAssets(ctx, 1, "a1", "import", replacement, false)
	require.ErrorIs(t, err, ErrImportConfirmation)
	_, err = fx.store.db.ExecContext(ctx, `CREATE TRIGGER reject_asset_import BEFORE INSERT ON artifact_assets WHEN NEW.id='new' BEGIN SELECT RAISE(ABORT, 'injected failure'); END`)
	require.NoError(t, err)
	_, err = fx.store.ImportArtifactAssets(ctx, 1, "a1", "import", replacement, true)
	require.Error(t, err)
	got, err := fx.store.ListArtifactAssets(ctx, 1, "a1")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "old-blob", got[0].BlobID)
}
