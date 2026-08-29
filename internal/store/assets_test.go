package store

// Out-of-line asset lifecycle (av-20fk). These pin the two claims the design
// rests on and one it deliberately does not make:
//
//   - a superseded generation's bytes are reclaimed, so a repeated refetch
//     cannot accumulate a full asset set every time it runs;
//   - a blob two artifacts share survives the deletion of either one, which is
//     what per-owner content addressing costs and what makes the refcount
//     load-bearing rather than defensive;
//   - and nothing here ever consults the artifact's body to decide any of it.

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedAssetArtifact writes an artifact with a body blob, so asset tests start from a
// row assets can hang off.
func seedAssetArtifact(t *testing.T, fx *queueFixture, id string) {
	t.Helper()
	fx.putBody(t, id+"-body", "<html><script>fetch('/app.wasm')</script></html>")
	require.NoError(t, fx.store.PutArtifact(context.Background(), &Artifact{
		ID: id, OwnerID: 1, Title: id, SourceBlobID: id + "-body", Tier: Tier1,
	}))
}

func asset(id, sourceURL, blobID string) ArtifactAsset {
	return ArtifactAsset{
		ID: id, SourceURL: sourceURL, BlobID: blobID,
		ContentType: "application/wasm", SizeBytes: 8,
	}
}

func TestReplacingAGenerationReclaimsTheSupersededBytes(t *testing.T) {
	fx := newQueueFixture(t)
	ctx := context.Background()
	seedAssetArtifact(t, fx, "a1")

	fx.putBody(t, "wasm-v1", "\x00asm-one")
	_, err := fx.store.ReplaceArtifactAssets(ctx, 1, "a1", "gen-1",
		[]ArtifactAsset{asset("as-1", "https://cdn.test/app.wasm", "wasm-v1")})
	require.NoError(t, err)

	// A refetch that brings back different bytes: new generation, and the old
	// set is deletable as a unit because the body it belonged to is gone too.
	fx.putBody(t, "wasm-v2", "\x00asm-two")
	queued, err := fx.store.ReplaceArtifactAssets(ctx, 1, "a1", "gen-2",
		[]ArtifactAsset{asset("as-2", "https://cdn.test/app.wasm", "wasm-v2")})
	require.NoError(t, err)
	assert.Equal(t, []string{"wasm-v1"}, queued)

	n, err := fx.store.DrainBlobDeletions(ctx, fx.blobs, queued)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	assert.NoFileExists(t, fx.path("wasm-v1"))
	require.FileExists(t, fx.path("wasm-v2"))

	// One live generation, not two: this is what stops a repeated refetch
	// growing the library without bound.
	assets, err := fx.store.ListArtifactAssets(ctx, 1, "a1")
	require.NoError(t, err)
	require.Len(t, assets, 1)
	assert.Equal(t, "gen-2", assets[0].GenerationID)
}

// The common refetch: the upstream payload has not changed. Content addressing
// gives the new generation the same blob id, so nothing is condemned and the
// bytes are never touched — which is why generations need no pruning policy.
func TestAnUnchangedPayloadSurvivesItsGenerationBeingReplaced(t *testing.T) {
	fx := newQueueFixture(t)
	ctx := context.Background()
	seedAssetArtifact(t, fx, "a1")

	fx.putBody(t, "wasm-stable", "\x00asm-stable")
	_, err := fx.store.ReplaceArtifactAssets(ctx, 1, "a1", "gen-1",
		[]ArtifactAsset{asset("as-1", "https://cdn.test/app.wasm", "wasm-stable")})
	require.NoError(t, err)

	queued, err := fx.store.ReplaceArtifactAssets(ctx, 1, "a1", "gen-2",
		[]ArtifactAsset{asset("as-2", "https://cdn.test/app.wasm", "wasm-stable")})
	require.NoError(t, err)
	assert.Empty(t, queued, "the new generation still names those bytes")

	pending, err := fx.store.PendingBlobDeletions(ctx)
	require.NoError(t, err)
	assert.Empty(t, pending)
	require.FileExists(t, fx.path("wasm-stable"))
}

// Per-owner content addressing means one library's two artifacts that load the
// same wasm share a blob. Deleting either must leave the other working — this
// is the case an unconditional enqueue would silently break.
func TestAnAssetBlobSharedByTwoArtifactsOutlivesTheFirstDeletion(t *testing.T) {
	fx := newQueueFixture(t)
	ctx := context.Background()
	seedAssetArtifact(t, fx, "a1")
	seedAssetArtifact(t, fx, "a2")

	fx.putBody(t, "shared-wasm", "\x00asm-shared")
	for artifactID, assetID := range map[string]string{"a1": "as-a1", "a2": "as-a2"} {
		_, err := fx.store.ReplaceArtifactAssets(ctx, 1, artifactID, "gen-"+artifactID,
			[]ArtifactAsset{asset(assetID, "https://cdn.test/app.wasm", "shared-wasm")})
		require.NoError(t, err)
	}

	queued, err := fx.store.DeleteArtifact(ctx, 1, "a1")
	require.NoError(t, err)
	assert.NotContains(t, queued, "shared-wasm", "a2 still loads those bytes")

	pending, err := fx.store.PendingBlobDeletions(ctx)
	require.NoError(t, err)
	assert.NotContains(t, pending, "shared-wasm")

	_, err = fx.store.DrainAllBlobDeletions(ctx, fx.blobs)
	require.NoError(t, err)
	require.FileExists(t, fx.path("shared-wasm"), "the survivor's payload must still be there")

	surviving, err := fx.store.ListArtifactAssets(ctx, 1, "a2")
	require.NoError(t, err)
	require.Len(t, surviving, 1)

	// The second deletion takes the last reference with it.
	queued, err = fx.store.DeleteArtifact(ctx, 1, "a2")
	require.NoError(t, err)
	assert.Contains(t, queued, "shared-wasm")
	_, err = fx.store.DrainBlobDeletions(ctx, fx.blobs, queued)
	require.NoError(t, err)
	assert.NoFileExists(t, fx.path("shared-wasm"))
}

// A queued id records that nothing referenced those bytes *when they were
// condemned*, which stops being true if the same payload is ingested again
// before the drain runs: asset blobs are content-addressed, so the new asset
// row names the very id sitting in the queue. A drain that took the queue at
// its word would delete the bytes out from under a live artifact — and this is
// not a narrow window, because a drain that fails leaves its row for the next
// startup, however many ingests later that is.
func TestADrainKeepsBytesThatWereReferencedAgainAfterBeingQueued(t *testing.T) {
	fx := newQueueFixture(t)
	ctx := context.Background()
	seedAssetArtifact(t, fx, "a1")
	seedAssetArtifact(t, fx, "a2")

	fx.putBody(t, "wasm-shared", "\x00asm-payload")
	_, err := fx.store.ReplaceArtifactAssets(ctx, 1, "a1", "gen-1",
		[]ArtifactAsset{asset("as-1", "https://cdn.test/app.wasm", "wasm-shared")})
	require.NoError(t, err)

	queued, err := fx.store.DeleteArtifact(ctx, 1, "a1")
	require.NoError(t, err)
	require.Contains(t, queued, "wasm-shared")

	// The re-ingest lands in the gap: same bytes, same owner, therefore the
	// same content address the queue is still holding.
	fx.putBody(t, "wasm-shared", "\x00asm-payload")
	_, err = fx.store.ReplaceArtifactAssets(ctx, 1, "a2", "gen-1",
		[]ArtifactAsset{asset("as-2", "https://cdn.test/app.wasm", "wasm-shared")})
	require.NoError(t, err)

	n, err := fx.store.DrainBlobDeletions(ctx, fx.blobs, queued)
	require.NoError(t, err)
	require.FileExists(t, fx.path("wasm-shared"), "a2 loads those bytes now")
	assert.NoFileExists(t, fx.path("a1-body"), "the genuinely unreferenced body still goes")
	assert.Equal(t, len(queued), n, "the reprieved id leaves the queue too")

	// Retired rather than left to be reconsidered on every later drain.
	pending, err := fx.store.PendingBlobDeletions(ctx)
	require.NoError(t, err)
	assert.Empty(t, pending)

	// And it is a reprieve, not an amnesty: once a2 goes, the bytes go.
	queued, err = fx.store.DeleteArtifact(ctx, 1, "a2")
	require.NoError(t, err)
	require.Contains(t, queued, "wasm-shared")
	_, err = fx.store.DrainBlobDeletions(ctx, fx.blobs, queued)
	require.NoError(t, err)
	assert.NoFileExists(t, fx.path("wasm-shared"))
}

// Deleting the artifact takes its assets' bytes, which requires reading the
// blob ids before the cascade removes the rows that name them.
func TestDeletingAnArtifactReclaimsItsAssetBytes(t *testing.T) {
	fx := newQueueFixture(t)
	ctx := context.Background()
	seedAssetArtifact(t, fx, "a1")

	fx.putBody(t, "only-wasm", "\x00asm-only")
	_, err := fx.store.ReplaceArtifactAssets(ctx, 1, "a1", "gen-1",
		[]ArtifactAsset{asset("as-1", "https://cdn.test/app.wasm", "only-wasm")})
	require.NoError(t, err)

	queued, err := fx.store.DeleteArtifact(ctx, 1, "a1")
	require.NoError(t, err)
	assert.Contains(t, queued, "only-wasm")
	assert.Contains(t, queued, "a1-body")

	_, err = fx.store.DrainBlobDeletions(ctx, fx.blobs, queued)
	require.NoError(t, err)
	assert.NoFileExists(t, fx.path("only-wasm"))
}

// The owner's escape hatch for the one case no rule can decide.
func TestDeletingOneAssetReclaimsOnlyItsBytes(t *testing.T) {
	fx := newQueueFixture(t)
	ctx := context.Background()
	seedAssetArtifact(t, fx, "a1")

	fx.putBody(t, "wasm-a", "\x00asm-a")
	fx.putBody(t, "wasm-b", "\x00asm-b")
	_, err := fx.store.ReplaceArtifactAssets(ctx, 1, "a1", "gen-1", []ArtifactAsset{
		asset("as-a", "https://cdn.test/a.wasm", "wasm-a"),
		asset("as-b", "https://cdn.test/b.wasm", "wasm-b"),
	})
	require.NoError(t, err)

	queued, err := fx.store.DeleteArtifactAsset(ctx, 1, "a1", "as-a")
	require.NoError(t, err)
	assert.Equal(t, []string{"wasm-a"}, queued)
	_, err = fx.store.DrainBlobDeletions(ctx, fx.blobs, queued)
	require.NoError(t, err)

	assert.NoFileExists(t, fx.path("wasm-a"))
	require.FileExists(t, fx.path("wasm-b"))

	remaining, err := fx.store.ListArtifactAssets(ctx, 1, "a1")
	require.NoError(t, err)
	require.Len(t, remaining, 1)
	assert.Equal(t, "as-b", remaining[0].ID)
}

// Another owner's artifact and one that never existed are the same answer, as
// everywhere else on this interface.
func TestAssetWritesAreOwnerScoped(t *testing.T) {
	fx := newQueueFixture(t)
	ctx := context.Background()
	seedAssetArtifact(t, fx, "a1")

	_, err := fx.store.ReplaceArtifactAssets(ctx, 2, "a1", "gen-1",
		[]ArtifactAsset{asset("as-1", "https://cdn.test/app.wasm", "wasm")})
	assert.ErrorIs(t, err, ErrNotFound)

	_, err = fx.store.DeleteArtifactAsset(ctx, 2, "a1", "as-1")
	assert.ErrorIs(t, err, ErrNotFound)

	assets, err := fx.store.ListArtifactAssets(ctx, 2, "a1")
	require.NoError(t, err)
	assert.Empty(t, assets)
}

// An asset is addressable only through the artifact that owns it, which is what
// keeps the un-tokened render route from becoming a way to read across
// artifacts for anyone who learns an id.
func TestAnAssetIsNotReachableThroughAnotherArtifact(t *testing.T) {
	fx := newQueueFixture(t)
	ctx := context.Background()
	seedAssetArtifact(t, fx, "a1")
	seedAssetArtifact(t, fx, "a2")

	fx.putBody(t, "wasm-a", "\x00asm-a")
	_, err := fx.store.ReplaceArtifactAssets(ctx, 1, "a1", "gen-1",
		[]ArtifactAsset{asset("as-1", "https://cdn.test/app.wasm", "wasm-a")})
	require.NoError(t, err)

	found, err := fx.store.GetArtifactAssetUnscoped(ctx, "a1", "as-1")
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, "wasm-a", found.BlobID)

	crossed, err := fx.store.GetArtifactAssetUnscoped(ctx, "a2", "as-1")
	require.NoError(t, err)
	assert.Nil(t, crossed, "the artifact id in the path must gate the lookup")
}
