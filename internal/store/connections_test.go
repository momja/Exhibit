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
	require.NoError(t, s.CreateConnection(ctx, &ConnectionRecord{ID: "quota", OwnerID: alice, Name: "Bank", Provider: "basic", Kind: "native", RecipeJSON: "{}", EncryptedSecret: "sealed"}, -1))
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

func TestConnectionGrantRequiresViewerOwnershipAndRejectsDelegation(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	require.NoError(t, s.PutArtifact(ctx, &Artifact{ID: "connected", OwnerID: alice, Title: "Bank", Tier: Tier1, SourceBlobID: "body"}))
	for _, c := range []*ConnectionRecord{{ID: "owner", OwnerID: alice, Name: "Owner", Provider: "bearer", Kind: "native", RecipeJSON: "{}", EncryptedSecret: "sealed"}, {ID: "viewer", OwnerID: bob, Name: "Viewer", Provider: "bearer", Kind: "native", RecipeJSON: "{}", EncryptedSecret: "sealed"}} {
		require.NoError(t, s.CreateConnection(ctx, c, -1))
	}
	assert.ErrorIs(t, s.PutConnectionGrant(ctx, ConnectionGrantRecord{ArtifactID: "connected", ConnectionID: "owner", ViewerID: bob}), ErrNotFound)
	for _, grant := range []ConnectionGrantRecord{
		{ArtifactID: "connected", ConnectionID: "owner", ViewerID: alice, ShareWithRecipients: true},
		{ArtifactID: "connected", ConnectionID: "viewer", ViewerID: bob, ShareWithRecipients: true},
	} {
		assert.ErrorIs(t, s.PutConnectionGrant(ctx, grant), ErrConnectionSharing)
	}
	require.NoError(t, s.PutConnectionGrant(ctx, ConnectionGrantRecord{ArtifactID: "connected", ConnectionID: "owner", ViewerID: alice}))
	// Even an un-upgraded or directly restored legacy row cannot delegate.
	_, err := s.db.ExecContext(ctx, `UPDATE artifact_connection_grants SET share_with_recipients=1`)
	require.NoError(t, err)
	grants, err := s.ListConnectionGrants(ctx, bob, "connected")
	require.NoError(t, err)
	assert.Empty(t, grants)
	_, err = s.GetGrantedConnection(ctx, bob, "connected", "owner")
	assert.ErrorIs(t, err, ErrNotFound)
	require.NoError(t, s.PutConnectionGrant(ctx, ConnectionGrantRecord{ArtifactID: "connected", ConnectionID: "viewer", ViewerID: bob}))
	grants, err = s.ListConnectionGrants(ctx, bob, "connected")
	require.NoError(t, err)
	require.Len(t, grants, 1)
	assert.Equal(t, "viewer", grants[0].ConnectionID)
	// A corrupted grant naming somebody else's connection also grants nothing.
	_, err = s.db.ExecContext(ctx, `INSERT INTO artifact_connection_grants(artifact_id,connection_id,viewer_id) VALUES ('connected','owner',?)`, bob)
	require.NoError(t, err)
	grants, err = s.ListConnectionGrants(ctx, bob, "connected")
	require.NoError(t, err)
	require.Len(t, grants, 1)
	_, err = s.GetGrantedConnection(ctx, bob, "connected", "owner")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestSavedResultSharingNeverDelegatesConnectionAccess(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	require.NoError(t, s.PutArtifact(ctx, &Artifact{ID: "personal-connected", OwnerID: alice, Title: "Private", SourceBlobID: "body", Tier: Tier1}))
	require.NoError(t, s.CreateConnection(ctx, &ConnectionRecord{ID: "personal-connection", OwnerID: alice, Name: "Bank", Provider: "basic", Kind: "native", RecipeJSON: "{}", EncryptedSecret: "sealed"}, -1))
	grant := ConnectionGrantRecord{ArtifactID: "personal-connected", ConnectionID: "personal-connection", ViewerID: alice}
	require.NoError(t, s.PutConnectionGrant(ctx, grant))
	require.NoError(t, s.UpdateArtifact(ctx, alice, "personal-connected", map[string]any{"share_state_mode": ShareStateShared}))
	_, err := s.CreateAnonymousLink(ctx, alice, "personal-connected")
	require.NoError(t, err)
	_, err = s.ReplaceAnonymousLink(ctx, alice, "personal-connected", "public-rotated")
	require.NoError(t, err)
	require.NoError(t, s.PutConnectionGrant(ctx, grant))
	grants, err := s.ListConnectionGrants(ctx, bob, "personal-connected")
	require.NoError(t, err)
	assert.Empty(t, grants)
	_, err = s.GetGrantedConnection(ctx, bob, "personal-connected", "personal-connection")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestConnectionOriginOverlapIsAtomicAndPersonal(t *testing.T) {
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
	assert.ErrorIs(t, s.PutConnectionGrant(ctx, ConnectionGrantRecord{ArtifactID: "origin-collision", ConnectionID: grants[0].ConnectionID, ViewerID: alice, ShareWithRecipients: true}), ErrConnectionSharing)
	viewerGrants, err := s.ListConnectionGrants(ctx, bob, "origin-collision")
	require.NoError(t, err)
	require.Len(t, viewerGrants, 1)
	assert.Equal(t, "viewer", viewerGrants[0].ConnectionID, "matching origins do not collide across different people's accounts")
}
