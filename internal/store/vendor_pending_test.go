package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPendingVendorCleanupSurvivesUnexpectedAccountErasure(t *testing.T) {
	fx := seedEverything(t, newTestStore(t))
	ctx := context.Background()
	require.NoError(t, fx.s.CreatePendingVendorConnection(ctx, PendingVendorConnection{
		ID: "unfinished", OwnerID: fx.member.ID, RequestJSON: `{"ExternalRef":"remote-account"}`, ExpiresAt: time.Now().Add(time.Hour),
	}))
	_, err := fx.s.DeleteAccount(ctx, fx.member.ID)
	require.NoError(t, err)
	// Normal API erasure first drains this queue. A lower-level deletion or
	// race must still leave enough ownership for maintenance to revoke it.
	rows, err := fx.s.ListPendingVendorConnections(ctx, 0, time.Time{})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, fx.member.ID, rows[0].OwnerID)
	assert.Contains(t, rows[0].RequestJSON, "remote-account")
}
