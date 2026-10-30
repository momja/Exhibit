package store

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserDisplayName(t *testing.T) {
	for _, tc := range []struct {
		user      User
		name      string
		isSubject bool
	}{
		{User{Email: "a@b.test", ExternalID: "sub-1"}, "a@b.test", false},
		{User{ExternalID: "sub-1"}, "sub-1", true},
		{User{Email: "alice", ExternalID: "local:alice"}, "alice", false},
		{User{ExternalID: "local:alice"}, "alice", false},
		{User{}, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			name, isSubject := tc.user.DisplayName()
			assert.Equal(t, tc.name, name)
			assert.Equal(t, tc.isSubject, isSubject)
		})
	}
}

func TestSharedListingsScopesAndOwnerNames(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	seedOwnerAccounts(t, s)
	local, err := s.UpsertUser(ctx, "local:carol", "")
	require.NoError(t, err)
	subject, err := s.UpsertUser(ctx, "provider-subject", "")
	require.NoError(t, err)
	// Same display name, distinct owners: grouping must use the owner ID.
	twin, err := s.UpsertUser(ctx, "another-subject", "sub-alice@example.test")
	require.NoError(t, err)
	put := func(owner int64, id string, recipients ...int64) {
		t.Helper()
		require.NoError(t, s.PutArtifact(ctx, &Artifact{
			ID: id, OwnerID: owner, Title: id, SourceBlobID: "blob-" + id, Tier: Tier1,
			CreatedAt:        time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			NetworkAllowlist: []string{"https://example.test"},
		}))
		for _, recipient := range recipients {
			_, err := s.CreateGrant(ctx, owner, id, recipient)
			require.NoError(t, err)
		}
	}
	put(alice, "alice-1", bob, local.ID)
	put(alice, "alice-2", bob)
	put(local.ID, "local", bob)
	put(subject.ID, "subject", bob)
	put(twin.ID, "twin", bob)
	put(alice, "private")
	put(alice, "other-recipient", local.ID)
	put(alice, "public-only")
	_, err = s.CreateAnonymousLink(ctx, alice, "public-only")
	require.NoError(t, err)
	put(bob, "self-grant", bob) // A legacy/raw self-grant is still excluded.
	put(bob, "outgoing", alice, local.ID)
	_, err = s.CreateAnonymousLink(ctx, bob, "outgoing")
	require.NoError(t, err)

	listed, err := s.ListArtifactsSharedWith(ctx, ViewerID(bob), SharedListOptions{})
	require.NoError(t, err)
	require.Len(t, listed, 5)
	assert.Equal(t, []string{"twin", "subject", "local", "alice-2", "alice-1"}, sharedArtifactIDs(listed))
	for _, result := range listed {
		readable, err := s.GetArtifactReadableBy(ctx, ViewerID(bob), result.ID)
		require.NoError(t, err)
		require.NotNil(t, readable)
		assert.Equal(t, readable, result.Artifact, "list hydrates the same metadata as the viewer accessor")
		owner, err := s.GetUser(ctx, result.OwnerID)
		require.NoError(t, err)
		name, _ := owner.DisplayName()
		assert.Equal(t, name, result.OwnerName)
	}
	owners, err := s.SharedWithOwners(ctx, ViewerID(bob))
	require.NoError(t, err)
	assert.Equal(t, []SharedOwner{
		{ID: local.ID, Name: "carol", Count: 1},
		{ID: subject.ID, Name: "provider-subject", Count: 1},
		{ID: alice, Name: "sub-alice@example.test", Count: 2},
		{ID: twin.ID, Name: "sub-alice@example.test", Count: 1},
	}, owners)
	with, by, err := s.SpaceCounts(ctx, ViewerID(bob))
	require.NoError(t, err)
	assert.Equal(t, 5, with)
	assert.Equal(t, 2, by, "an artifact with several grants and a link counts once")

	for _, tc := range []struct {
		owners []int64
		want   []string
	}{
		{[]int64{alice}, []string{"alice-2", "alice-1"}},
		{[]int64{alice, local.ID, alice}, []string{"local", "alice-2", "alice-1"}},
		{[]int64{bob, 999}, nil},
	} {
		got, err := s.ListArtifactsSharedWith(ctx, ViewerID(bob), SharedListOptions{OwnerIDs: tc.owners})
		require.NoError(t, err)
		assert.Equal(t, tc.want, sharedArtifactIDs(got))
	}
	for _, query := range []string{"private", "other-recipient", "public-only", "self-grant", "outgoing"} {
		got, err := s.ListArtifactsSharedWith(ctx, ViewerID(bob), SharedListOptions{Query: query})
		require.NoError(t, err)
		assert.Empty(t, got, "an FTS match must not admit an artifact without an incoming grant")
	}
	filtered, err := s.ListArtifactsSharedWith(ctx, ViewerID(bob), SharedListOptions{OwnerIDs: []int64{alice}, Query: "alice", Limit: 1, Offset: 1})
	require.NoError(t, err)
	assert.Equal(t, []string{"alice-1"}, sharedArtifactIDs(filtered))
	for _, viewer := range []ViewerID{0, 999} {
		got, err := s.ListArtifactsSharedWith(ctx, viewer, SharedListOptions{})
		require.NoError(t, err)
		assert.Empty(t, got)
		owners, err := s.SharedWithOwners(ctx, viewer)
		require.NoError(t, err)
		assert.Empty(t, owners)
		with, by, err := s.SpaceCounts(ctx, viewer)
		require.NoError(t, err)
		assert.Zero(t, with)
		assert.Zero(t, by)
	}
}

func TestSharedListingsFollowGrantLifecycle(t *testing.T) {
	for _, change := range []string{"revoked grant", "deleted artifact", "deleted recipient", "disabled owner"} {
		t.Run(change, func(t *testing.T) {
			f := newGrantFixture(t)
			ctx := t.Context()
			_, err := f.s.CreateGrant(ctx, alice, f.artifactID, bob)
			require.NoError(t, err)
			before, err := f.s.ListArtifactsSharedWith(ctx, ViewerID(bob), SharedListOptions{})
			require.NoError(t, err)
			require.Len(t, before, 1)
			switch change {
			case "revoked grant":
				require.NoError(t, f.s.DeleteGrant(ctx, alice, f.artifactID, bob))
			case "deleted artifact":
				_, err = f.s.DeleteArtifact(ctx, alice, f.artifactID)
				require.NoError(t, err)
			case "deleted recipient":
				_, err = f.s.DeleteAccount(ctx, bob)
				require.NoError(t, err)
			case "disabled owner":
				require.NoError(t, f.s.SetUserAdmin(ctx, bob, true))
				require.NoError(t, f.s.SetUserDisabled(ctx, alice, true))
			}
			want := 0
			if change == "disabled owner" {
				want = 1 // Disabling login does not revoke existing grants.
			}
			after, err := f.s.ListArtifactsSharedWith(ctx, ViewerID(bob), SharedListOptions{})
			require.NoError(t, err)
			assert.Len(t, after, want)
			readable, err := f.s.GetArtifactReadableBy(ctx, ViewerID(bob), f.artifactID)
			require.NoError(t, err)
			assert.Equal(t, want == 1, readable != nil)
			owners, err := f.s.SharedWithOwners(ctx, ViewerID(bob))
			require.NoError(t, err)
			assert.Len(t, owners, want)
			with, _, err := f.s.SpaceCounts(ctx, ViewerID(bob))
			require.NoError(t, err)
			assert.Equal(t, want, with)
			_, by, err := f.s.SpaceCounts(ctx, ViewerID(alice))
			require.NoError(t, err)
			assert.Equal(t, want, by)
		})
	}
}

func TestSharedListingSearchAndPagingMatchOwnedListing(t *testing.T) {
	f := newGrantFixture(t)
	ctx := t.Context()
	_, err := f.s.DeleteArtifact(ctx, alice, f.artifactID)
	require.NoError(t, err)
	for i, title := range []string{"Bar Chart Builder", "Bar Chalk Board", "Notepad"} {
		id := fmt.Sprintf("search-%d", i)
		require.NoError(t, f.s.PutArtifact(ctx, &Artifact{
			ID: id, OwnerID: alice, Title: title, Tier: Tier1, SourceBlobID: "blob-" + id,
			SourceText: "sourceword", CreatedAt: time.Date(2026, 1, 1, i, 0, 0, 0, time.UTC),
		}))
		_, err := f.s.CreateGrant(ctx, alice, id, bob)
		require.NoError(t, err)
	}
	tag := &Tag{ID: "tag", OwnerID: alice, Name: "gadget"}
	require.NoError(t, f.s.CreateTag(ctx, tag))
	require.NoError(t, f.s.AddArtifactTag(ctx, alice, "search-0", tag.ID))
	for _, query := range []string{"", "cha", "bar chart", "bar notepad", "sourceword", "gadget", "<chart>", "<>", `" OR 1=1 --`} {
		t.Run(query, func(t *testing.T) {
			for _, page := range []struct{ limit, offset int }{{0, 0}, {1, 0}, {1, 1}, {1, 9}} {
				owned, err := f.s.ListArtifacts(ctx, ListOptions{OwnerID: alice, Query: query, Limit: page.limit, Offset: page.offset})
				require.NoError(t, err)
				shared, err := f.s.ListArtifactsSharedWith(ctx, ViewerID(bob), SharedListOptions{Query: query, Limit: page.limit, Offset: page.offset})
				require.NoError(t, err)
				var artifacts []*Artifact
				for _, result := range shared {
					artifacts = append(artifacts, result.Artifact)
				}
				assert.Equal(t, owned, artifacts)
			}
		})
	}
}

func TestSharedOnlyKeepsTheOwnerScopeAndFilters(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	seedOwnerAccounts(t, s)
	for _, owner := range []int64{alice, bob} {
		for _, kind := range []string{"private", "link", "grant", "both"} {
			id := fmt.Sprintf("%d-%s", owner, kind)
			require.NoError(t, s.PutArtifact(ctx, &Artifact{
				ID: id, OwnerID: owner, Title: kind + " chart", SourceBlobID: id, Tier: Tier1,
				CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			}))
			if kind == "link" || kind == "both" {
				_, err := s.CreateAnonymousLink(ctx, owner, id)
				require.NoError(t, err)
			}
			if kind == "grant" || kind == "both" {
				_, err := s.CreateGrant(ctx, owner, id, 3-owner)
				require.NoError(t, err)
			}
		}
		owned, err := s.ListArtifacts(ctx, ListOptions{OwnerID: owner, SharedOnly: true})
		require.NoError(t, err)
		assert.Len(t, owned, 3)
		var ids []string
		for _, a := range owned {
			ids = append(ids, a.ID)
			assert.Equal(t, owner, a.OwnerID)
			assert.True(t, a.SharePublicLink || a.ShareGrantCount > 0)
		}
		assert.ElementsMatch(t, []string{fmt.Sprintf("%d-link", owner), fmt.Sprintf("%d-grant", owner), fmt.Sprintf("%d-both", owner)}, ids)
		filtered, err := s.ListArtifacts(ctx, ListOptions{OwnerID: owner, SharedOnly: true, Query: "grant"})
		require.NoError(t, err)
		require.Len(t, filtered, 1)
		assert.Equal(t, fmt.Sprintf("%d-grant", owner), filtered[0].ID)
		with, by, err := s.SpaceCounts(ctx, ViewerID(owner))
		require.NoError(t, err)
		// The other owner's rows may not have been created yet.
		assert.Equal(t, int(owner-1)*2, with)
		assert.Equal(t, 3, by)
	}
	tag := &Tag{ID: "shared-tag", OwnerID: alice, Name: "work"}
	require.NoError(t, s.CreateTag(ctx, tag))
	collection := &Collection{ID: "shared-collection", OwnerID: alice, Name: "work"}
	require.NoError(t, s.CreateCollection(ctx, collection))
	for _, id := range []string{"1-private", "1-link", "1-grant"} {
		require.NoError(t, s.AddArtifactTag(ctx, alice, id, tag.ID))
		require.NoError(t, s.AddArtifactToCollection(ctx, alice, id, collection.ID))
	}
	filtered, err := s.ListArtifacts(ctx, ListOptions{OwnerID: alice, SharedOnly: true, Tags: []string{"work"}, Collections: []string{"work"}, Query: "chart", Limit: 1, Offset: 1})
	require.NoError(t, err)
	require.Len(t, filtered, 1)
	assert.Equal(t, "1-grant", filtered[0].ID)
	got, err := s.ListArtifacts(ctx, ListOptions{SharedOnly: true})
	require.NoError(t, err)
	assert.Empty(t, got)
}

func sharedArtifactIDs(artifacts []*SharedArtifact) []string {
	var ids []string
	for _, a := range artifacts {
		ids = append(ids, a.ID)
	}
	return ids
}
