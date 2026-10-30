package store

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConnectionRequestsAreScopedAndIdempotent(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	putVersioned(t, s, "app")
	request, err := s.RequestArtifactConnection(ctx, 1, "app", "basic", "Read account balances")
	require.NoError(t, err)
	retry, err := s.RequestArtifactConnection(ctx, 1, "app", "basic", "Another reason")
	require.NoError(t, err)
	assert.Equal(t, request.ID, retry.ID)
	assert.Equal(t, request.Reason, retry.Reason)
	_, err = s.RequestArtifactConnection(ctx, 2, "app", "bearer", "Wrong owner")
	assert.ErrorIs(t, err, ErrNotFound)
	other, err := s.ListArtifactConnectionRequests(ctx, 2, "app")
	require.NoError(t, err)
	assert.Empty(t, other)
	require.NoError(t, s.DeleteArtifactConnectionRequest(ctx, 2, "app", request.ID))
	current, err := s.ListArtifactConnectionRequests(ctx, 1, "app")
	require.NoError(t, err)
	require.Len(t, current, 1)
	assert.Equal(t, request.CreatedAt, current[0].CreatedAt)
	require.NoError(t, s.DeleteArtifactConnectionRequest(ctx, 1, "app", request.ID))
	current, err = s.ListArtifactConnectionRequests(ctx, 1, "app")
	require.NoError(t, err)
	assert.Empty(t, current)
}

func TestConnectionRequestsAreBoundedAndCascade(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	putVersioned(t, s, "app")
	for i := 0; i < 8; i++ {
		_, err := s.RequestArtifactConnection(ctx, 1, "app", fmt.Sprintf("provider-%d", i), "An account is needed")
		require.NoError(t, err)
	}
	_, err := s.RequestArtifactConnection(ctx, 1, "app", "extra", "No more prompts")
	assert.ErrorIs(t, err, ErrConnectionRequestLimit)
	_, err = s.RequestArtifactConnection(ctx, 1, "app", "provider-0", "Idempotent at capacity")
	require.NoError(t, err)
	_, err = s.DeleteArtifact(ctx, 1, "app")
	require.NoError(t, err)
	var count int
	require.NoError(t, s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM artifact_connection_requests`).Scan(&count))
	assert.Zero(t, count)
}

func TestPersonalGrantResolvesOnlyMatchingOwnerRequest(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	putVersioned(t, s, "app")
	for _, provider := range []string{"bearer", "future_service"} {
		_, err := s.RequestArtifactConnection(ctx, 1, "app", provider, "Connect this account")
		require.NoError(t, err)
	}
	for _, account := range []struct {
		owner int64
		id    string
	}{{2, "recipient"}, {1, "owner"}} {
		putPolicyConnection(t, s, account.owner, account.id, "https://api.example.test")
		require.NoError(t, s.PutConnectionGrant(ctx, ConnectionGrantRecord{ArtifactID: "app", ConnectionID: account.id, ViewerID: account.owner}))
		requests, err := s.ListArtifactConnectionRequests(ctx, 1, "app")
		require.NoError(t, err)
		if account.owner == 2 {
			assert.Len(t, requests, 2, "recipient setup cannot dismiss the owner's request")
		} else {
			require.Len(t, requests, 1)
			assert.Equal(t, "future_service", requests[0].Provider)
		}
	}
}
