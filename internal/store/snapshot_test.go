package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSnapshotAssetFailureRollsBackBodyAndDeletionQueue(t *testing.T) {
	fx := newQueueFixture(t)
	ctx := t.Context()
	seedAssetArtifact(t, fx, "a1")
	fx.putBody(t, "old-asset", "old payload")
	_, err := fx.store.ReplaceArtifactAssets(ctx, 1, "a1", "old-generation",
		[]ArtifactAsset{asset("old-id", "https://cdn.test/a.wasm", "old-asset")})
	require.NoError(t, err)
	before, err := fx.store.GetArtifact(ctx, 1, "a1")
	require.NoError(t, err)
	assetsBefore, err := fx.store.ListArtifactAssets(ctx, 1, "a1")
	require.NoError(t, err)
	fx.putBody(t, "new-body", "new body")
	fx.putBody(t, "new-asset", "new payload")

	// The first asset insert succeeds; the second violates the primary key.
	// Both asset changes and the preceding body update must roll back.
	_, err = fx.store.ReplaceArtifactSnapshot(ctx, 1, "a1", ArtifactSnapshot{
		SourceBlobID: "new-body", SourceText: "new text", GenerationID: "new-generation",
		Assets: []ArtifactAsset{
			asset("duplicate", "https://cdn.test/a.wasm", "new-asset"),
			asset("duplicate", "https://cdn.test/b.wasm", "new-asset"),
		},
	})
	require.Error(t, err)
	after, err := fx.store.GetArtifact(ctx, 1, "a1")
	require.NoError(t, err)
	assert.Equal(t, before, after)
	assetsAfter, err := fx.store.ListArtifactAssets(ctx, 1, "a1")
	require.NoError(t, err)
	assert.Equal(t, assetsBefore, assetsAfter)
	n, err := fx.store.DrainAllBlobDeletions(ctx, fx.blobs)
	require.NoError(t, err)
	assert.Zero(t, n)
	require.FileExists(t, fx.path("a1-body"))
	require.FileExists(t, fx.path("old-asset"))

	queued, err := fx.store.ReplaceArtifactSnapshot(ctx, 1, "a1", ArtifactSnapshot{
		SourceBlobID: "new-body", SourceText: "new text", GenerationID: "new-generation",
		Assets: []ArtifactAsset{asset("new-id", "https://cdn.test/a.wasm", "new-asset")},
	})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"a1-body", "old-asset"}, queued)
	after, err = fx.store.GetArtifact(ctx, 1, "a1")
	require.NoError(t, err)
	assert.Equal(t, "new-body", after.SourceBlobID)
	var sourceText string
	require.NoError(t, fx.store.db.QueryRowContext(ctx, "SELECT source_text FROM artifacts WHERE id=?", "a1").Scan(&sourceText))
	assert.Equal(t, "new text", sourceText)
	assetsAfter, err = fx.store.ListArtifactAssets(ctx, 1, "a1")
	require.NoError(t, err)
	require.Len(t, assetsAfter, 1)
	assert.Equal(t, "new-id", assetsAfter[0].ID)
	fx.restart(t)
	n, err = fx.store.DrainAllBlobDeletions(ctx, fx.blobs)
	require.NoError(t, err)
	assert.Equal(t, 2, n)
	assert.NoFileExists(t, fx.path("a1-body"))
	assert.NoFileExists(t, fx.path("old-asset"))
	require.FileExists(t, fx.path("new-body"))
	require.FileExists(t, fx.path("new-asset"))
}
