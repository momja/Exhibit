package store

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	f, err := os.CreateTemp("", "test-*.db")
	require.NoError(t, err)
	f.Close()
	t.Cleanup(func() { os.Remove(f.Name()) })

	s, err := OpenSQLite(f.Name())
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	return s
}

func TestPutGetArtifact(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	a := &Artifact{
		ID:               "test-id-1",
		OwnerID:          1,
		Title:            "Test Artifact",
		SourceBlobID:     "blob-1",
		Tier:             Tier1,
		NetworkAllowlist: []string{"https://cdn.example.com"},
	}

	err := s.PutArtifact(ctx, a)
	require.NoError(t, err)

	got, err := s.GetArtifact(ctx, "test-id-1")
	require.NoError(t, err)
	require.NotNil(t, got)

	assert.Equal(t, a.ID, got.ID)
	assert.Equal(t, a.Title, got.Title)
	assert.Equal(t, a.NetworkAllowlist, got.NetworkAllowlist)
}

func TestListArtifacts(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	for i, title := range []string{"foo chart", "bar graph", "foo table"} {
		err := s.PutArtifact(ctx, &Artifact{
			ID:           fmt.Sprintf("id-%d", i),
			OwnerID:      1,
			Title:        title,
			SourceBlobID: fmt.Sprintf("blob-%d", i),
			Tier:         Tier1,
		})
		require.NoError(t, err)
	}

	all, err := s.ListArtifacts(ctx, ListOptions{})
	require.NoError(t, err)
	assert.Len(t, all, 3)
}

func TestState(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	err := s.PutArtifact(ctx, &Artifact{
		ID: "state-test", OwnerID: 1, Title: "T", SourceBlobID: "b", Tier: Tier1,
	})
	require.NoError(t, err)

	state, err := s.GetState(ctx, "state-test")
	require.NoError(t, err)
	assert.Empty(t, state)

	err = s.SetState(ctx, "state-test", "key1", "value1")
	require.NoError(t, err)

	state, err = s.GetState(ctx, "state-test")
	require.NoError(t, err)
	assert.Equal(t, "value1", state["key1"])

	// Upsert
	err = s.SetState(ctx, "state-test", "key1", "updated")
	require.NoError(t, err)
	state, _ = s.GetState(ctx, "state-test")
	assert.Equal(t, "updated", state["key1"])
}

func TestCollectionsAndTags(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	err := s.PutArtifact(ctx, &Artifact{
		ID: "a1", OwnerID: 1, Title: "Artifact 1", SourceBlobID: "b1", Tier: Tier1,
	})
	require.NoError(t, err)

	col := &Collection{ID: "col1", OwnerID: 1, Name: "My Collection"}
	err = s.CreateCollection(ctx, col)
	require.NoError(t, err)

	err = s.AddArtifactToCollection(ctx, "a1", "col1")
	require.NoError(t, err)

	tag := &Tag{ID: "tag1", OwnerID: 1, Name: "charts"}
	err = s.CreateTag(ctx, tag)
	require.NoError(t, err)

	err = s.AddArtifactTag(ctx, "a1", "tag1")
	require.NoError(t, err)

	cols, err := s.ListCollections(ctx, 1)
	require.NoError(t, err)
	assert.Len(t, cols, 1)

	tags, err := s.ListTags(ctx, 1)
	require.NoError(t, err)
	assert.Len(t, tags, 1)
}

func TestCreateTagColor(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Explicit color is persisted verbatim.
	err := s.CreateTag(ctx, &Tag{ID: "tag-red", OwnerID: 1, Name: "urgent", Color: "#FF0000"})
	require.NoError(t, err)

	// Omitted color falls back to the default.
	defaulted := &Tag{ID: "tag-default", OwnerID: 1, Name: "misc"}
	err = s.CreateTag(ctx, defaulted)
	require.NoError(t, err)
	assert.Equal(t, DefaultTagColor, defaulted.Color, "CreateTag should backfill the default color on the passed tag")

	tags, err := s.ListTags(ctx, 1)
	require.NoError(t, err)
	require.Len(t, tags, 2)

	colors := map[string]string{}
	for _, tag := range tags {
		colors[tag.ID] = tag.Color
	}
	assert.Equal(t, "#FF0000", colors["tag-red"])
	assert.Equal(t, DefaultTagColor, colors["tag-default"])
}
