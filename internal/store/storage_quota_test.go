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
}
