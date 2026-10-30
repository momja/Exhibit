package store

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLibraryKitIsPerOwnerAndValidated(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	got, err := s.LibraryKit(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, "", got, "no choice until one is made")

	require.NoError(t, s.SetLibraryKit(ctx, 1, "paper/v1"))
	require.NoError(t, s.SetLibraryKit(ctx, 1, "brutal/v1"))
	got, _ = s.LibraryKit(ctx, 1)
	assert.Equal(t, "brutal/v1", got, "a second choice replaces the first")
	other, _ := s.LibraryKit(ctx, 2)
	assert.Equal(t, "", other, "another owner's library is untouched")

	assert.Error(t, s.SetLibraryKit(ctx, 1, "neon/v1"))
	require.NoError(t, s.SetLibraryKit(ctx, 1, ""))
	got, _ = s.LibraryKit(ctx, 1)
	assert.Equal(t, "", got)
}

func TestArtifactKitIsValidated(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	a := &Artifact{ID: "a", OwnerID: 1, Title: "A", SourceBlobID: "b", Tier: 1}
	require.NoError(t, s.PutArtifact(ctx, a))

	require.NoError(t, s.UpdateArtifact(ctx, 1, "a", map[string]any{"kit": "paper/v1"}))
	got, err := s.GetArtifact(ctx, 1, "a")
	require.NoError(t, err)
	assert.Equal(t, "paper/v1", got.Kit)

	assert.Error(t, s.UpdateArtifact(ctx, 1, "a", map[string]any{"kit": "neon/v1"}))
	assert.Error(t, s.UpdateArtifact(ctx, 1, "a", map[string]any{"kit": 7}))
}
