package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func importedBundleFixture() ArtifactBundleImport {
	return ArtifactBundleImport{ID: "imported", Title: "Imported title", Tier: Tier2,
		SourceURL: "https://example.com/tool", BodyBlobID: "fresh-source", WidgetBlobID: "fresh-widget",
		SourceText: "importedsearchterm", GenerationID: "fresh-generation",
		Assets: []ArtifactAsset{{ID: "fresh-asset", BlobID: "fresh-payload", SourceURL: "https://example.com/image", ContentType: "image/png", SizeBytes: 5}},
		State:  map[string]string{"": "", "__proto__": "literal", "todos": "[1,  2]", "unicode": "世界\n"}}
}

func TestImportArtifactBundlePublishesPrivateOwnerSnapshotAndInitialVersion(t *testing.T) {
	s := newTestStore(t)
	seedOwnerAccounts(t, s)
	bundle := importedBundleFixture()
	require.NoError(t, s.ImportArtifactBundle(t.Context(), bob, bundle))
	a, err := s.GetArtifact(t.Context(), bob, bundle.ID)
	require.NoError(t, err)
	require.NotNil(t, a)
	assert.Equal(t, bob, a.OwnerID)
	assert.Equal(t, bundle.Title, a.Title)
	assert.Equal(t, bundle.SourceURL, a.SourceURL)
	assert.Equal(t, bundle.Tier, a.Tier)
	assert.False(t, a.DownloadsApproved || a.ClipboardApproved || a.LinksApproved || a.CameraApproved || a.MicrophoneApproved || a.GeolocationApproved)
	assert.Empty(t, a.NetworkAllowlist)
	assert.False(t, a.SharePublicLink)
	assert.Zero(t, a.ShareGrantCount)
	assert.Equal(t, ShareStateOwn, a.ShareStateMode)
	assert.Equal(t, 1, a.VersionSeq)
	assert.Empty(t, a.Tags)
	other, err := s.GetArtifact(t.Context(), alice, bundle.ID)
	require.NoError(t, err)
	assert.Nil(t, other)
	state, err := s.GetState(t.Context(), OwnerID(bob), bundle.ID, ViewerID(bob))
	require.NoError(t, err)
	assert.Equal(t, bundle.State, state)
	state, err = s.GetState(t.Context(), OwnerID(bob), bundle.ID, ViewerID(alice))
	require.NoError(t, err)
	assert.Empty(t, state)
	version, err := s.GetVersion(t.Context(), bob, bundle.ID, 1)
	require.NoError(t, err)
	assert.Equal(t, bundle.BodyBlobID, version.BodyBlobID)
	assert.Equal(t, bundle.WidgetBlobID, version.WidgetBlobID)
	assert.Equal(t, VersionInitial, version.Origin)
	matches, err := s.ListArtifacts(t.Context(), ListOptions{OwnerID: bob, Query: "importedsearchterm"})
	require.NoError(t, err)
	require.Len(t, matches, 1)
	_, err = s.CommitVersion(t.Context(), bob, bundle.ID, VersionChange{BodyBlobID: strPtr("next-source")})
	require.NoError(t, err)
	historical, err := s.VersionAssets(t.Context(), bob, bundle.ID, 1)
	require.NoError(t, err)
	require.Len(t, historical, 1)
	assert.Equal(t, bundle.Assets[0].ID, historical[0].ID)
	assert.Equal(t, bundle.Assets[0].BlobID, historical[0].BlobID)
	view, err := s.GetVersionView(t.Context(), bob, bundle.ID, 1)
	require.NoError(t, err)
	assert.Equal(t, bundle.State, view.State)
}

func TestImportArtifactBundleRollsBackAllRowsOnLateFailure(t *testing.T) {
	for _, failure := range []string{"asset", "state"} {
		t.Run(failure, func(t *testing.T) {
			s := newTestStore(t)
			seedOwnerAccounts(t, s)
			bundle := importedBundleFixture()
			if failure == "asset" {
				bundle.Assets = append(bundle.Assets, bundle.Assets[0])
			} else {
				_, err := s.db.Exec(`CREATE TRIGGER fail_import_state BEFORE INSERT ON artifact_state BEGIN SELECT RAISE(ABORT, 'simulated state failure'); END`)
				require.NoError(t, err)
			}
			require.Error(t, s.ImportArtifactBundle(t.Context(), alice, bundle))
			for _, table := range []string{"artifacts", "artifact_versions", "artifact_assets", "artifact_version_assets", "artifact_state", "asset_identities", "artifact_network_origins", "shares"} {
				var count int
				require.NoError(t, s.db.QueryRow("SELECT COUNT(*) FROM "+table).Scan(&count))
				assert.Zero(t, count, table)
			}
			matches, err := s.ListArtifacts(t.Context(), ListOptions{OwnerID: alice, Query: "importedsearchterm"})
			require.NoError(t, err)
			assert.Empty(t, matches, "FTS trigger changes roll back too")
		})
	}
}

func TestImportArtifactBundleRequiresEnabledAccountAndFreshIdentities(t *testing.T) {
	s := newTestStore(t)
	seedOwnerAccounts(t, s)
	bundle := importedBundleFixture()
	require.ErrorIs(t, s.ImportArtifactBundle(t.Context(), 999, bundle), ErrNotFound)
	require.NoError(t, s.SetUserDisabled(t.Context(), bob, true))
	require.ErrorIs(t, s.ImportArtifactBundle(t.Context(), bob, bundle), ErrNotFound)
	require.NoError(t, s.ImportArtifactBundle(t.Context(), alice, bundle))
	require.Error(t, s.ImportArtifactBundle(t.Context(), alice, bundle), "a retry cannot overwrite existing content")
	bundle.ID = "second"
	require.ErrorIs(t, s.ImportArtifactBundle(t.Context(), alice, bundle), ErrImportAssetConflict)
	_, err := s.DeleteArtifact(t.Context(), alice, "imported")
	require.NoError(t, err)
	require.ErrorIs(t, s.ImportArtifactBundle(t.Context(), alice, bundle), ErrImportAssetConflict, "deleted immutable identities cannot be reassigned")
	bundle.State = make(map[string]string, MaxImportedStateKeys+1)
	for i := 0; i <= MaxImportedStateKeys; i++ {
		bundle.State[string(rune(i))] = "value"
	}
	require.ErrorIs(t, s.ImportArtifactBundle(t.Context(), alice, bundle), ErrImportStateTooLarge)
}
