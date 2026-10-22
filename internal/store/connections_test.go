package store

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConnectionEntitlementsAreAtomicAndOwnerScoped(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	record := func(id string, owner int64) *ConnectionRecord {
		return &ConnectionRecord{ID: id, OwnerID: owner, Name: "personal", Provider: "bearer", Kind: "native", RecipeJSON: `{"origins":["https://api.example.test"]}`, EncryptedSecret: "sealed-credential"}
	}
	require.NoError(t, s.CreateConnection(ctx, record("one", alice), 1))
	assert.ErrorIs(t, s.CreateConnection(ctx, record("two", alice), 1), ErrConnectionLimit)
	require.NoError(t, s.CreateConnection(ctx, record("two", bob), 1))
	_, err := s.GetConnection(ctx, bob, "one")
	assert.ErrorIs(t, err, ErrNotFound)
	assert.ErrorIs(t, s.DeleteConnection(ctx, bob, "one"), ErrNotFound)
	assert.ErrorIs(t, s.UpdateConnectionSecret(ctx, bob, "one", "evil"), ErrNotFound)
	require.NoError(t, s.DeleteConnection(ctx, alice, "one"))
	require.NoError(t, s.CreateConnection(ctx, record("three", alice), 1))
	raw, err := json.Marshal(record("safe", alice))
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "sealed-credential")
	assert.NotContains(t, string(raw), "https://api.example.test")
}

func TestConnectionQuotaReservationSurvivesConcurrentRequests(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	require.NoError(t, s.CreateConnection(ctx, &ConnectionRecord{ID: "quota", OwnerID: alice, Name: "Bank", Provider: "simplefin", Kind: "native", RecipeJSON: "{}", EncryptedSecret: "sealed"}, -1))
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	var mu sync.Mutex
	successes := 0
	var wg sync.WaitGroup
	for i := 0; i < 80; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := s.ReserveConnectionRequest(ctx, "quota", 24, now)
			if err == nil {
				mu.Lock()
				successes++
				mu.Unlock()
			} else {
				assert.ErrorIs(t, err, ErrConnectionQuota)
			}
		}()
	}
	wg.Wait()
	assert.Equal(t, 24, successes)
	n, err := s.ConnectionRequestsToday(ctx, "quota", now)
	require.NoError(t, err)
	assert.Equal(t, 24, n)
	require.NoError(t, s.ReserveConnectionRequest(ctx, "quota", 24, now.Add(24*time.Hour)))
}

func TestConnectionGrantRequiresViewerOwnershipAndExplicitOwnerSharing(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	require.NoError(t, s.PutArtifact(ctx, &Artifact{ID: "connected", OwnerID: alice, Title: "Bank", Tier: Tier1, SourceBlobID: "body"}))
	for _, c := range []*ConnectionRecord{{ID: "owner", OwnerID: alice, Name: "Owner", Provider: "bearer", Kind: "native", RecipeJSON: "{}", EncryptedSecret: "sealed"}, {ID: "viewer", OwnerID: bob, Name: "Viewer", Provider: "bearer", Kind: "native", RecipeJSON: "{}", EncryptedSecret: "sealed"}} {
		require.NoError(t, s.CreateConnection(ctx, c, -1))
	}
	assert.ErrorIs(t, s.PutConnectionGrant(ctx, ConnectionGrantRecord{ArtifactID: "connected", ConnectionID: "owner", ViewerID: bob}), ErrNotFound)
	assert.ErrorIs(t, s.PutConnectionGrant(ctx, ConnectionGrantRecord{ArtifactID: "connected", ConnectionID: "viewer", ViewerID: bob, ShareWithRecipients: true}), ErrNotFound)
	require.NoError(t, s.PutConnectionGrant(ctx, ConnectionGrantRecord{ArtifactID: "connected", ConnectionID: "owner", ViewerID: alice}))
	grants, err := s.ListConnectionGrants(ctx, bob, "connected")
	require.NoError(t, err)
	assert.Empty(t, grants)
	_, err = s.GetGrantedConnection(ctx, bob, "connected", "owner")
	assert.ErrorIs(t, err, ErrNotFound)
	require.NoError(t, s.PutConnectionGrant(ctx, ConnectionGrantRecord{ArtifactID: "connected", ConnectionID: "owner", ViewerID: alice, ShareWithRecipients: true}))
	grants, err = s.ListConnectionGrants(ctx, bob, "connected")
	require.NoError(t, err)
	require.Len(t, grants, 1)
	require.NoError(t, s.DeleteConnection(ctx, alice, "owner"))
	grants, err = s.ListConnectionGrants(ctx, bob, "connected")
	require.NoError(t, err)
	assert.Empty(t, grants)
}

func TestConnectionSharingInvariantAlsoGuardsStoreMutations(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	require.NoError(t, s.PutArtifact(ctx, &Artifact{ID: "private-connected", OwnerID: alice, Title: "Private", SourceBlobID: "body", Tier: Tier1}))
	require.NoError(t, s.CreateConnection(ctx, &ConnectionRecord{ID: "private-connection", OwnerID: alice, Name: "Bank", Provider: "simplefin", Kind: "native", RecipeJSON: "{}", EncryptedSecret: "sealed"}, -1))
	grant := ConnectionGrantRecord{ArtifactID: "private-connected", ConnectionID: "private-connection", ViewerID: alice}
	require.NoError(t, s.PutConnectionGrant(ctx, grant))
	assert.ErrorIs(t, s.UpdateArtifact(ctx, alice, "private-connected", map[string]any{"share_state_mode": ShareStateShared}), ErrConnectionSharing)
	_, shareErr := s.CreateAnonymousLink(ctx, alice, "private-connected")
	assert.ErrorIs(t, shareErr, ErrConnectionSharing)
	_, err := s.ReplaceAnonymousLink(ctx, alice, "private-connected", "public-rotated")
	assert.ErrorIs(t, err, ErrConnectionSharing)
	grant.ShareWithRecipients = true
	require.NoError(t, s.PutConnectionGrant(ctx, grant))
	_, shareErr = s.CreateAnonymousLink(ctx, alice, "private-connected")
	require.NoError(t, shareErr)
	require.NoError(t, s.UpdateArtifact(ctx, alice, "private-connected", map[string]any{"share_state_mode": ShareStateShared}))
	grant.ShareWithRecipients = false
	assert.ErrorIs(t, s.PutConnectionGrant(ctx, grant), ErrNotFound)
}

func TestConnectionOriginOverlapIsAtomicAndIncludesSharedOwnerBindings(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	require.NoError(t, s.PutArtifact(ctx, &Artifact{ID: "origin-collision", OwnerID: alice, Title: "App", SourceBlobID: "body", Tier: Tier1}))
	for _, c := range []*ConnectionRecord{{ID: "a", OwnerID: alice}, {ID: "b", OwnerID: alice}, {ID: "viewer", OwnerID: bob}} {
		c.Name = c.ID
		c.Provider = "bearer"
		c.Kind = "native"
		c.RecipeJSON = `{"origins":["https://api.example.test"]}`
		c.EncryptedSecret = "sealed"
		require.NoError(t, s.CreateConnection(ctx, c, -1))
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	successes := 0
	for _, id := range []string{"a", "b"} {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			err := s.PutConnectionGrant(ctx, ConnectionGrantRecord{ArtifactID: "origin-collision", ConnectionID: id, ViewerID: alice})
			if err == nil {
				mu.Lock()
				successes++
				mu.Unlock()
			} else {
				assert.ErrorIs(t, err, ErrConnectionOverlap)
			}
		}(id)
	}
	wg.Wait()
	assert.Equal(t, 1, successes)
	grants, err := s.ListConnectionGrants(ctx, alice, "origin-collision")
	require.NoError(t, err)
	require.Len(t, grants, 1)
	require.NoError(t, s.PutConnectionGrant(ctx, ConnectionGrantRecord{ArtifactID: "origin-collision", ConnectionID: "viewer", ViewerID: bob}))
	assert.ErrorIs(t, s.PutConnectionGrant(ctx, ConnectionGrantRecord{ArtifactID: "origin-collision", ConnectionID: grants[0].ConnectionID, ViewerID: alice, ShareWithRecipients: true}), ErrConnectionOverlap)
	require.NoError(t, s.DeleteConnectionGrant(ctx, bob, "origin-collision", "viewer"))
	require.NoError(t, s.PutConnectionGrant(ctx, ConnectionGrantRecord{ArtifactID: "origin-collision", ConnectionID: grants[0].ConnectionID, ViewerID: alice, ShareWithRecipients: true}))
	assert.ErrorIs(t, s.PutConnectionGrant(ctx, ConnectionGrantRecord{ArtifactID: "origin-collision", ConnectionID: "viewer", ViewerID: bob}), ErrConnectionOverlap)
}
