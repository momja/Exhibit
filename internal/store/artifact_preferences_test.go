package store

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPersonalStarsAndOpenOrder(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	seedOwnerAccounts(t, s)
	for _, id := range []string{"older", "newer", "private"} {
		require.NoError(t, s.PutArtifact(ctx, &Artifact{ID: id, OwnerID: alice, Title: id, SourceBlobID: "body", Tier: Tier1}))
	}
	for _, id := range []string{"older", "newer"} {
		_, err := s.CreateGrant(ctx, alice, id, bob)
		require.NoError(t, err)
		require.NoError(t, s.SetArtifactStarred(ctx, ViewerID(bob), id, true))
	}
	require.ErrorIs(t, s.SetArtifactStarred(ctx, ViewerID(bob), "private", true), ErrNotFound)
	require.ErrorIs(t, s.RecordArtifactOpened(ctx, ViewerID(bob), "private"), ErrNotFound)
	require.ErrorIs(t, s.SetArtifactStarred(ctx, ViewerID(0), "older", true), ErrNotFound)
	opts := ViewerGalleryOptions{StarredOnly: true, Limit: 1}
	owned, err := s.ListViewerGallery(ctx, ViewerID(alice), opts)
	require.NoError(t, err)
	assert.Empty(t, owned, "the owner's stars are independent")
	require.NoError(t, s.RecordArtifactOpened(ctx, ViewerID(bob), "older"))
	page, err := s.ListViewerGallery(ctx, ViewerID(bob), opts)
	require.NoError(t, err)
	require.Len(t, page, 1)
	assert.Equal(t, "older", page[0].ID)
	opts.Offset = 1
	page, err = s.ListViewerGallery(ctx, ViewerID(bob), opts)
	require.NoError(t, err)
	require.Len(t, page, 1)
	assert.Equal(t, "newer", page[0].ID)
	preferences, err := s.ArtifactPreferences(ctx, ViewerID(bob), []string{"older", "newer"})
	require.NoError(t, err)
	assert.True(t, preferences["older"].Starred)
	assert.WithinDuration(t, time.Now(), preferences["older"].LastOpenedAt, 5*time.Second)
	assert.True(t, preferences["newer"].LastOpenedAt.IsZero())
	opts.Offset = 0
	require.NoError(t, s.RecordArtifactOpened(ctx, ViewerID(bob), "newer"))
	page, err = s.ListViewerGallery(ctx, ViewerID(bob), opts)
	require.NoError(t, err)
	require.Len(t, page, 1)
	assert.Equal(t, "newer", page[0].ID, "a newer opening moves ahead of a previously opened favorite")
	require.NoError(t, s.RecordArtifactOpened(ctx, ViewerID(bob), "older"))
	page, err = s.ListViewerGallery(ctx, ViewerID(bob), opts)
	require.NoError(t, err)
	require.Len(t, page, 1)
	assert.Equal(t, "older", page[0].ID, "reopening changes the order again")
	require.NoError(t, s.SetArtifactStarred(ctx, ViewerID(bob), "older", false))
	preferences, err = s.ArtifactPreferences(ctx, ViewerID(bob), []string{"older"})
	require.NoError(t, err)
	assert.False(t, preferences["older"].Starred)
	assert.False(t, preferences["older"].LastOpenedAt.IsZero(), "unstar preserves opening history")
	require.NoError(t, s.DeleteGrant(ctx, alice, "newer", bob))
	opts.Offset = 0
	page, err = s.ListViewerGallery(ctx, ViewerID(bob), opts)
	require.NoError(t, err)
	assert.Empty(t, page, "revocation immediately hides a starred artifact")
	require.ErrorIs(t, s.SetArtifactStarred(ctx, ViewerID(bob), "newer", true), ErrNotFound)
}

func TestCombinedSharedGallery(t *testing.T) {
	s := newTestStore(t)
	seedOwnerAccounts(t, s)
	ctx := t.Context()
	for id, owner := range map[string]int64{"owned-private": bob, "owned-shared": bob, "incoming": alice, "foreign-private": alice, "foreign-public": alice} {
		require.NoError(t, s.PutArtifact(ctx, &Artifact{ID: id, OwnerID: owner, Title: "Chart " + id, SourceBlobID: "body", Tier: Tier1}))
	}
	_, err := s.CreateGrant(ctx, alice, "incoming", bob)
	require.NoError(t, err)
	_, err = s.CreateAnonymousLink(ctx, bob, "owned-shared")
	require.NoError(t, err)
	_, err = s.CreateAnonymousLink(ctx, alice, "foreign-public")
	require.NoError(t, err)
	all, err := s.ListViewerGallery(ctx, ViewerID(bob), ViewerGalleryOptions{SharedOnly: true, Limit: 24})
	require.NoError(t, err)
	require.Len(t, all, 2)
	ids := []string{all[0].ID, all[1].ID}
	assert.ElementsMatch(t, []string{"owned-shared", "incoming"}, ids)
	all, err = s.ListViewerGallery(ctx, ViewerID(bob), ViewerGalleryOptions{Limit: 24})
	require.NoError(t, err)
	assert.Len(t, all, 3, "empty search browses the complete accessible set")
}

func TestAccountDeletionErasesPersonalPreferencesOnReceivedArtifacts(t *testing.T) {
	s := newTestStore(t)
	seedOwnerAccounts(t, s)
	ctx := t.Context()
	require.NoError(t, s.PutArtifact(ctx, &Artifact{ID: "shared-tool", OwnerID: alice, Title: "Shared tool", SourceBlobID: "body", Tier: Tier1}))
	_, err := s.CreateGrant(ctx, alice, "shared-tool", bob)
	require.NoError(t, err)
	for _, viewer := range []int64{alice, bob} {
		require.NoError(t, s.SetArtifactStarred(ctx, ViewerID(viewer), "shared-tool", true))
		require.NoError(t, s.RecordArtifactOpened(ctx, ViewerID(viewer), "shared-tool"))
	}
	_, err = s.DeleteAccount(ctx, bob)
	require.NoError(t, err)
	erased, err := s.ArtifactPreferences(ctx, ViewerID(bob), []string{"shared-tool"})
	require.NoError(t, err)
	assert.Empty(t, erased, "deleting a viewer erases preferences even when the artifact survives")
	retained, err := s.ArtifactPreferences(ctx, ViewerID(alice), []string{"shared-tool"})
	require.NoError(t, err)
	assert.True(t, retained["shared-tool"].Starred, "another viewer's preference survives")
	_, err = s.DeleteArtifact(ctx, alice, "shared-tool")
	require.NoError(t, err)
	retained, err = s.ArtifactPreferences(ctx, ViewerID(alice), []string{"shared-tool"})
	require.NoError(t, err)
	assert.Empty(t, retained, "artifact deletion erases its remaining preferences")
}
