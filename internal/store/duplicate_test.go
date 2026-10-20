package store

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDuplicateArtifactOptionsAndIndependentMetadata(t *testing.T) {
	for _, copyShares := range []bool{false, true} {
		for _, copyData := range []bool{false, true} {
			t.Run(fmt.Sprintf("shares=%t/data=%t", copyShares, copyData), func(t *testing.T) {
				s := newTestStore(t)
				seedOwnerAccounts(t, s)
				ctx := context.Background()
				require.NoError(t, s.PutArtifact(ctx, &Artifact{ID: "source", OwnerID: alice, Title: "Original", SourceBlobID: "body", WidgetBlobID: "widget", SourceURL: "https://example.com/tool", Tier: Tier2, DownloadsApproved: true, ClipboardApproved: true, LinksApproved: true, CameraApproved: true, MicrophoneApproved: true, GeolocationApproved: true, NetworkAllowlist: []string{"https://allowed.example"}}))
				require.NoError(t, s.UpdateArtifact(ctx, alice, "source", map[string]any{"share_state_mode": ShareStateShared}))
				require.NoError(t, s.SetOriginDecision(ctx, alice, "source", "https://blocked.example", DecisionBlock, "user"))
				require.NoError(t, s.SetState(ctx, OwnerID(alice), "source", ViewerID(alice), "mine", "owner data"))
				require.NoError(t, s.SetState(ctx, OwnerID(alice), "source", ViewerID(bob), "private", "viewer data"))
				require.NoError(t, s.CreateTag(ctx, &Tag{ID: "tag", OwnerID: alice, Name: "tools"}))
				require.NoError(t, s.AddArtifactTag(ctx, alice, "source", "tag"))
				require.NoError(t, s.CreateCollection(ctx, &Collection{ID: "collection", OwnerID: alice, Name: "Work"}))
				require.NoError(t, s.AddArtifactToCollection(ctx, alice, "source", "collection"))
				require.NoError(t, s.CreateShare(ctx, alice, &Share{ID: "link", ArtifactID: "source"}))
				recipient := bob
				require.NoError(t, s.CreateShare(ctx, alice, &Share{ID: "grant", ArtifactID: "source", RecipientID: &recipient}))
				require.NoError(t, s.SaveTranscript(ctx, alice, Transcript{ArtifactID: "source", SessionID: "conversation", SessionFile: "{}\n"}))
				_, err := s.CommitVersion(ctx, alice, "source", VersionChange{BodyBlobID: strPtr("new-body")})
				require.NoError(t, err)
				d := ArtifactDuplicate{ID: "copy", Title: "My copy", BodyBlobID: "copy-body", WidgetBlobID: "copy-widget", SourceText: "searchablecopyterm", SourceVersion: 2, CopyData: copyData, CopyShareSettings: copyShares}
				created, err := s.DuplicateArtifact(ctx, alice, "source", d)
				require.NoError(t, err)
				require.True(t, created)
				a, err := s.GetArtifact(ctx, alice, "copy")
				require.NoError(t, err)
				require.NotNil(t, a)
				assert.Equal(t, "My copy", a.Title)
				assert.Equal(t, "https://example.com/tool", a.SourceURL)
				assert.Equal(t, Tier2, a.Tier)
				assert.True(t, a.DownloadsApproved && a.ClipboardApproved && a.LinksApproved && a.CameraApproved && a.MicrophoneApproved && a.GeolocationApproved)
				assert.Equal(t, []string{"https://allowed.example"}, a.NetworkAllowlist)
				require.Len(t, a.Tags, 1)
				assert.Equal(t, "tag", a.Tags[0].ID)
				matches, err := s.ListArtifacts(ctx, ListOptions{OwnerID: alice, Collections: []string{"Work"}, Query: "searchablecopyterm"})
				require.NoError(t, err)
				require.Len(t, matches, 1)
				assert.Equal(t, "copy", matches[0].ID)
				origins, err := s.ListOriginDecisions(ctx, alice, "copy")
				require.NoError(t, err)
				require.Len(t, origins, 2)
				assert.Equal(t, DecisionBlock, origins[1].Decision)
				state, err := s.GetState(ctx, OwnerID(alice), "copy", ViewerID(alice))
				require.NoError(t, err)
				if copyData {
					assert.Equal(t, map[string]string{"mine": "owner data"}, state)
				} else {
					assert.Empty(t, state)
				}
				viewerState, err := s.GetState(ctx, OwnerID(alice), "copy", ViewerID(bob))
				require.NoError(t, err)
				assert.Empty(t, viewerState, "another viewer's private rows never travel")
				shares, err := s.ListArtifactShares(ctx, alice, "copy")
				require.NoError(t, err)
				if copyShares {
					require.Len(t, shares, 2)
					assert.Equal(t, ShareStateShared, a.ShareStateMode)
					assert.True(t, a.SharePublicLink)
					assert.Equal(t, 1, a.ShareGrantCount)
					for _, sh := range shares {
						assert.NotContains(t, []string{"link", "grant"}, sh.ID)
					}
				} else {
					assert.Empty(t, shares)
					assert.Equal(t, ShareStateOwn, a.ShareStateMode)
					assert.False(t, a.SharePublicLink)
					assert.Zero(t, a.ShareGrantCount)
				}
				versions, err := s.ListVersions(ctx, alice, "copy")
				require.NoError(t, err)
				require.Len(t, versions, 1)
				assert.Equal(t, VersionInitial, versions[0].Origin)
				assert.Empty(t, versions[0].SessionID)
				assert.False(t, versions[0].HasState)
				conversations, err := s.ListTranscripts(ctx, alice, "copy")
				require.NoError(t, err)
				assert.Empty(t, conversations)
				require.NoError(t, s.SetState(ctx, OwnerID(alice), "copy", ViewerID(alice), "mine", "changed"))
				state, err = s.GetState(ctx, OwnerID(alice), "source", ViewerID(alice))
				require.NoError(t, err)
				assert.Equal(t, "owner data", state["mine"])
				_, err = s.DeleteArtifact(ctx, alice, "source")
				require.NoError(t, err)
				// Same identity is a receipt after deletion and never overwrites edits.
				d.Title = "Overwritten"
				created, err = s.DuplicateArtifact(ctx, alice, "source", d)
				require.NoError(t, err)
				assert.False(t, created)
				a, err = s.GetArtifact(ctx, alice, "copy")
				require.NoError(t, err)
				assert.Equal(t, "My copy", a.Title)
			})
		}
	}
}

func TestDuplicateArtifactRefusesForeignStaleAndPartialClones(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	putVersioned(t, s, "source")
	d := ArtifactDuplicate{ID: "copy", Title: "Copy", BodyBlobID: "copy-body", SourceVersion: 1}
	for _, sourceID := range []string{"source", "missing"} {
		created, err := s.DuplicateArtifact(ctx, bob, sourceID, d)
		assert.False(t, created)
		assert.ErrorIs(t, err, ErrNotFound)
	}
	_, err := s.CommitVersion(ctx, alice, "source", VersionChange{BodyBlobID: strPtr("body2")})
	require.NoError(t, err)
	_, err = s.DuplicateArtifact(ctx, alice, "source", d)
	assert.ErrorIs(t, err, ErrArtifactChanged)
	d.SourceVersion = 2
	// Force a late SQL constraint error after the artifact insert; every earlier
	// row (including its initial version) must roll back with the asset failure.
	d.Assets = []ArtifactAsset{{ID: "same"}, {ID: "same"}}
	_, err = s.DuplicateArtifact(ctx, alice, "source", d)
	require.Error(t, err)
	a, err := s.GetArtifact(ctx, alice, "copy")
	require.NoError(t, err)
	assert.Nil(t, a)
	versions, err := s.ListVersions(ctx, alice, "copy")
	require.NoError(t, err)
	assert.Empty(t, versions)
	d.Assets = nil
	_, err = s.ReplaceArtifactAssets(ctx, alice, "source", "generation", []ArtifactAsset{{ID: "asset", BlobID: "bytes", SourceURL: "https://example.com/image.png"}})
	require.NoError(t, err)
	_, err = s.DuplicateArtifact(ctx, alice, "source", d)
	assert.ErrorIs(t, err, ErrArtifactChanged, "an asset change without a version change also refuses stale assembly")
}

func TestGalleryOrdersMixedAndSubsecondCreationTimesChronologically(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	for _, id := range []string{"z-whole-second", "a-fractional", "sql-older"} {
		putVersioned(t, s, id)
	}
	for id, stamp := range map[string]string{"z-whole-second": "2026-10-20T12:00:00Z", "a-fractional": "2026-10-20T12:00:00.5Z", "sql-older": "2026-10-20 11:59:59"} {
		_, err := s.db.Exec(`UPDATE artifacts SET created_at=? WHERE id=?`, stamp, id)
		require.NoError(t, err)
	}
	artifacts, err := s.ListArtifacts(ctx, ListOptions{OwnerID: 1})
	require.NoError(t, err)
	require.Len(t, artifacts, 3)
	assert.Equal(t, "a-fractional", artifacts[0].ID)
	assert.Equal(t, "z-whole-second", artifacts[1].ID)
	assert.Equal(t, "sql-older", artifacts[2].ID)
}
