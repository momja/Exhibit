package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStorageProjectionDeduplicatesWithinEachOwner(t *testing.T) {
	s := newTestStore(t)
	putSized(t, s, "mine", 1, "shared", 100, "", 0)
	putSized(t, s, "also-mine", 1, "shared", 100, "", 0)
	putSized(t, s, "theirs", 2, "foreign", 25, "", 0)
	blobs := map[string]int64{"shared": 100, "foreign": 25, "new": 10}
	current, additional, err := s.ProjectStorageUsage(context.Background(), 1, blobs)
	require.NoError(t, err)
	assert.Equal(t, int64(100), current)
	assert.Equal(t, int64(35), additional, "other owners' references do not make storage free")
	current, additional, err = s.ProjectStorageUsage(context.Background(), 2, blobs)
	require.NoError(t, err)
	assert.Equal(t, int64(25), current)
	assert.Equal(t, int64(110), additional)
	assert.Equal(t, int64(100), usage(t, s, 1), "projection must not change accounting")
}

func TestStorageProjectionRefusesUnknownUsageOnlyForItsOwner(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	require.NoError(t, s.PutArtifact(ctx, &Artifact{ID: "legacy", OwnerID: 1, SourceBlobID: "unmeasured", Tier: Tier1}))
	_, _, err := s.ProjectStorageUsage(ctx, 1, map[string]int64{"new": 1})
	require.ErrorContains(t, err, "no recorded size")
	current, additional, err := s.ProjectStorageUsage(ctx, 2, map[string]int64{"new": 1})
	require.NoError(t, err)
	assert.Zero(t, current)
	assert.Equal(t, int64(1), additional)
	_, _, err = s.ProjectAssetReplacementUsage(ctx, 1, "legacy", nil)
	require.ErrorContains(t, err, "no recorded size")
}

func TestAssetReplacementProjectionMatchesTheCompleteSwap(t *testing.T) {
	for _, tc := range []struct {
		name  string
		blobs map[string]int64
		want  int64
	}{
		{"empty", nil, 68},
		{"new smaller set", map[string]int64{"new": 3}, 71},
		{"reuse retired payload", map[string]int64{"retired": 10}, 78},
		{"shared and historical payloads count once", map[string]int64{"new": 3, "shared": 7, "history": 11, "foreign": 13}, 84},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestStore(t)
			ctx := t.Context()
			putSized(t, s, "mine", 1, "body", 20, "", 0)
			require.NoError(t, s.RecordBlobSize(ctx, "history", 11))
			_, err := s.ReplaceArtifactAssets(ctx, 1, "mine", "historical", []ArtifactAsset{asset("historical", "https://example.test/old", "history")})
			require.NoError(t, err)
			require.NoError(t, s.RecordBlobSize(ctx, "body2", 30))
			_, err = s.CommitVersion(ctx, 1, "mine", VersionChange{BodyBlobID: strPtr("body2")})
			require.NoError(t, err)
			putSized(t, s, "also-mine", 1, "shared", 7, "", 0)
			putSized(t, s, "theirs", 2, "foreign", 13, "", 0)
			require.NoError(t, s.RecordBlobSize(ctx, "retired", 10))
			original := []ArtifactAsset{
				asset("old-one", "https://example.test/one", "retired"),
				asset("old-two", "https://example.test/two", "retired"),
				asset("shared", "https://example.test/shared", "shared"),
				asset("body", "https://example.test/body", "body"),
				asset("history", "https://example.test/history", "history"),
			}
			_, err = s.ReplaceArtifactAssets(ctx, 1, "mine", "current", original)
			require.NoError(t, err)
			current, additional, err := s.ProjectAssetReplacementUsage(ctx, 1, "mine", tc.blobs)
			require.NoError(t, err)
			assert.Equal(t, int64(78), current)
			assert.Equal(t, tc.want, current+additional)
			assert.Equal(t, current, usage(t, s, 1), "projection must not change the saved set")
			var replacement []ArtifactAsset
			for id, size := range tc.blobs {
				require.NoError(t, s.RecordBlobSize(ctx, id, size))
				replacement = append(replacement, asset("import-"+id, "https://example.test/"+id, id))
			}
			_, err = s.ImportArtifactAssets(ctx, 1, "mine", "import", replacement, true)
			require.NoError(t, err)
			assert.Equal(t, tc.want, usage(t, s, 1), "the committed whole replacement must match its quota projection")
			assert.Equal(t, int64(13), usage(t, s, 2))
			got, err := s.ListArtifactAssets(ctx, 1, "mine")
			require.NoError(t, err)
			assert.Len(t, got, len(tc.blobs))
		})
	}
}
