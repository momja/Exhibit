package api

import (
	"context"
	"testing"

	"github.com/momja/Exhibit/internal/connectors"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelayPolicyRemainsValidWhenSharedResponseRetainsRecipientCustody(t *testing.T) {
	ro := newTestRouter(t)
	ro.cfg.Relay.Enabled = true
	s := ro.cfg.Store.(*store.SQLiteStore)
	ro.cfg.Connections = connectors.New(connectors.Config{Store: s, Box: ro.cfg.Secrets})
	ro.cfg.ConnectionPolicy = ro.cfg.Connections
	ctx := context.Background()
	owner, err := s.UpsertUser(ctx, "custody-owner", "owner@example.test")
	require.NoError(t, err)
	require.EqualValues(t, 1, owner.ID)
	recipient, err := s.UpsertUser(ctx, "custody-recipient", "recipient@example.test")
	require.NoError(t, err)
	id := createOwnArtifact(t, ro)
	_, err = s.CreateGrant(ctx, owner.ID, id, recipient.ID)
	require.NoError(t, err)
	for _, origin := range []string{"https://api.example.test", "https://old.example.test", "https://new.example.test"} {
		require.NoError(t, s.SetOriginDecision(ctx, owner.ID, id, origin, store.DecisionAllow, "test"))
	}
	connection, err := ro.cfg.Connections.Create(ctx, owner.ID, connectors.CreateRequest{Name: "Shared API", Recipe: "bearer", Origin: "https://api.example.test", Secret: "shared-secret"}, connectors.UnlimitedLimits())
	require.NoError(t, err)
	grant := connectors.GrantRequest{ConnectionID: connection.ID, ShareWithRecipients: true, PermittedEgress: []string{"https://old.example.test"}}
	_, err = ro.cfg.Connections.Grant(ctx, owner.ID, id, grant)
	require.NoError(t, err)
	a, err := s.GetArtifactReadableBy(ctx, store.ViewerID(recipient.ID), id)
	require.NoError(t, err)
	before, err := ro.relayPolicy(ctx, recipient.ID, a)
	require.NoError(t, err)
	require.NoError(t, s.RetainSharedConnectionDataPolicies(ctx, recipient.ID, id))
	after, err := ro.relayPolicy(ctx, recipient.ID, a)
	require.NoError(t, err)
	assert.Equal(t, before, after, "receiving the first response must not invalidate its own host before state save")

	grant.PermittedEgress = []string{"https://new.example.test"}
	_, err = ro.cfg.Connections.Grant(ctx, owner.ID, id, grant)
	require.NoError(t, err)
	before, err = ro.relayPolicy(ctx, recipient.ID, a)
	require.NoError(t, err)
	require.NoError(t, s.RetainSharedConnectionDataPolicies(ctx, recipient.ID, id))
	after, err = ro.relayPolicy(ctx, recipient.ID, a)
	require.NoError(t, err)
	assert.Equal(t, before, after, "narrowing a custody copy does not change the intersection already enforced")

	require.NoError(t, ro.cfg.Connections.Revoke(ctx, owner.ID, connection.ID))
	before, err = ro.relayPolicy(ctx, recipient.ID, a)
	require.NoError(t, err)
	require.NoError(t, ro.cfg.Connections.ClearData(ctx, owner.ID, id))
	after, err = ro.relayPolicy(ctx, recipient.ID, a)
	require.NoError(t, err)
	assert.Equal(t, before, after, "owner reset cannot alter restrictions on the recipient's retained copy")
}
