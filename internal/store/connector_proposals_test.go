package store

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func connectorProposalFixture(t *testing.T, s *SQLiteStore, id string) *ConnectorProposalRecord {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, s.PutArtifact(ctx, &Artifact{ID: id, OwnerID: alice, Title: "API client", SourceBlobID: "body", Tier: Tier1}))
	p, err := s.CreateConnectorProposal(ctx, &ConnectorProposalRecord{ID: "proposal-" + id, OwnerID: alice, ArtifactID: id, Name: "Weather", Origin: "https://api.weather.com", Auth: "api-key", Header: "X-Api-Key", Reason: "Fetch weather"})
	require.NoError(t, err)
	return p
}

func connectorProposalConnection(id string) *ConnectionRecord {
	return &ConnectionRecord{ID: id, OwnerID: alice, Name: "Weather", Provider: "api-key", Kind: "native", RecipeJSON: `{"origins":["https://api.weather.com"],"header":"X-Api-Key","scheme":""}`, EncryptedSecret: "sealed-" + id}
}

func TestConnectorProposalApprovalRollsBackConnectionOnDecisionFailure(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	p := connectorProposalFixture(t, s, "app")
	_, err := s.ResolveConnectorProposal(ctx, alice, "app", p.ID, true, connectorProposalConnection("over-quota"), 0)
	assert.ErrorIs(t, err, ErrConnectionLimit)
	pending, err := s.GetConnectorProposal(ctx, alice, "app", p.ID)
	require.NoError(t, err)
	assert.Equal(t, "pending", pending.State)
	// Force the second write to fail after inserting a new connection. Neither
	// the account nor an approved result may survive this failed transaction.
	_, err = s.db.ExecContext(ctx, `CREATE TRIGGER fail_connector_approval BEFORE UPDATE ON connector_proposals WHEN NEW.state='approved' BEGIN SELECT RAISE(ABORT,'fixture decision failure'); END`)
	require.NoError(t, err)
	_, err = s.ResolveConnectorProposal(ctx, alice, "app", p.ID, true, connectorProposalConnection("new"), -1)
	require.Error(t, err)
	connections, err := s.ListConnections(ctx, alice)
	require.NoError(t, err)
	assert.Empty(t, connections)
	saved, err := s.GetConnectorProposal(ctx, alice, "app", p.ID)
	require.NoError(t, err)
	assert.Equal(t, "pending", saved.State)
	_, err = s.db.ExecContext(ctx, `DROP TRIGGER fail_connector_approval`)
	require.NoError(t, err)
	_, err = s.ResolveConnectorProposal(ctx, alice, "app", p.ID, true, connectorProposalConnection("new"), 0)
	assert.ErrorIs(t, err, ErrConnectionLimit)
	saved, err = s.GetConnectorProposal(ctx, alice, "app", p.ID)
	require.NoError(t, err)
	assert.Equal(t, "pending", saved.State)
	approved, err := s.ResolveConnectorProposal(ctx, alice, "app", p.ID, true, connectorProposalConnection("new"), 1)
	require.NoError(t, err)
	assert.Equal(t, "approved", approved.State)
	assert.Equal(t, "new", approved.ConnectionID)
	// Replaying a committed operation needs no new credential or entitlement.
	retry, err := s.ResolveConnectorProposal(ctx, alice, "app", p.ID, true, nil, 0)
	require.NoError(t, err)
	assert.Equal(t, approved, retry)
}

func TestConnectorProposalRejectsDifferentCredentialDestination(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	p := connectorProposalFixture(t, s, "app")
	for _, policy := range []string{
		`{"origins":["https://evil.com"],"header":"X-Api-Key"}`,
		`{"origins":["https://api.weather.com","https://evil.com"],"header":"X-Api-Key"}`,
		`{"origins":["https://api.weather.com"],"header":"Authorization"}`,
	} {
		c := connectorProposalConnection("bad")
		c.RecipeJSON = policy
		_, err := s.ResolveConnectorProposal(ctx, alice, "app", p.ID, true, c, -1)
		assert.Error(t, err)
	}
	_, err := s.ResolveConnectorProposal(ctx, bob, "app", p.ID, true, connectorProposalConnection("bad"), -1)
	assert.ErrorIs(t, err, ErrNotFound)
	denied, err := s.ResolveConnectorProposal(ctx, alice, "app", p.ID, false, nil, -1)
	require.NoError(t, err)
	assert.Equal(t, "denied", denied.State)
	connections, err := s.ListConnections(ctx, alice)
	require.NoError(t, err)
	assert.Empty(t, connections)
}

func TestConnectorProposalsConcurrentAcrossArtifactsReuseOneConnection(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	proposals := make([]*ConnectorProposalRecord, 16)
	for i := range proposals {
		proposals[i] = connectorProposalFixture(t, s, fmt.Sprintf("app-%d", i))
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	results := make(chan *ConnectorProposalRecord, len(proposals))
	for i, p := range proposals {
		wg.Add(1)
		go func(i int, p *ConnectorProposalRecord) {
			defer wg.Done()
			<-start
			resolved, err := s.ResolveConnectorProposal(ctx, alice, p.ArtifactID, p.ID, true, connectorProposalConnection(fmt.Sprintf("account-%d", i)), 1)
			if assert.NoError(t, err) {
				results <- resolved
			}
		}(i, p)
	}
	close(start)
	wg.Wait()
	close(results)
	connections, err := s.ListConnections(ctx, alice)
	require.NoError(t, err)
	require.Len(t, connections, 1)
	for p := range results {
		assert.Equal(t, "approved", p.State)
		assert.Equal(t, connections[0].ID, p.ConnectionID)
	}
}

func TestConnectorProposalPendingLimitAndDenialIdempotency(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	p := connectorProposalFixture(t, s, "app")
	for i := 1; i < 8; i++ {
		candidate := *p
		candidate.ID, candidate.Origin = fmt.Sprintf("proposal-%d", i), fmt.Sprintf("https://api-%d.weather.com", i)
		_, err := s.CreateConnectorProposal(ctx, &candidate)
		require.NoError(t, err)
	}
	candidate := *p
	candidate.ID, candidate.Origin = "ninth", "https://api-9.weather.com"
	_, err := s.CreateConnectorProposal(ctx, &candidate)
	assert.ErrorIs(t, err, ErrConnectorProposalLimit)
	denied, err := s.ResolveConnectorProposal(ctx, alice, "app", p.ID, false, nil, -1)
	require.NoError(t, err)
	_, err = s.CreateConnectorProposal(ctx, &candidate)
	require.NoError(t, err, "denial frees a pending slot")
	retry := *p
	retry.ID, retry.Reason = "new-id", "Repeat request"
	repeated, err := s.CreateConnectorProposal(ctx, &retry)
	require.NoError(t, err)
	assert.Equal(t, denied, repeated)
}

func TestNativeCredentialDestinationDuplicateAndRevokedReplacement(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	c := connectorProposalConnection("existing")
	require.NoError(t, s.CreateConnection(ctx, c, -1))
	duplicate := connectorProposalConnection("duplicate")
	duplicate.RecipeJSON = `{"origins":["https://API.WEATHER.COM.:0443/"],"header":"x-api-key"}`
	assert.ErrorIs(t, s.CreateConnection(ctx, duplicate, -1), ErrConnectionDuplicate)
	otherOwner := *duplicate
	otherOwner.OwnerID = bob
	require.NoError(t, s.CreateConnection(ctx, &otherOwner, -1))
	otherHeader := connectorProposalConnection("other-header")
	otherHeader.RecipeJSON = `{"origins":["https://api.weather.com"],"header":"Authorization"}`
	require.NoError(t, s.CreateConnection(ctx, otherHeader, -1))
	_, err := s.db.ExecContext(ctx, `UPDATE connections SET status='revoked' WHERE id=?`, c.ID)
	require.NoError(t, err)
	duplicate.ID = "replacement"
	require.NoError(t, s.CreateConnection(ctx, duplicate, -1))
	saved, err := s.GetConnection(ctx, alice, c.ID)
	require.NoError(t, err)
	assert.Equal(t, c.EncryptedSecret, saved.EncryptedSecret, "replacement never changes a revoked key")
}

func TestConnectorProposalMigrationPreservesExistingDuplicateAccounts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "upgrade.db")
	s, err := OpenSQLite(path)
	require.NoError(t, err)
	require.NoError(t, goose.DownTo(s.db, "migrations", 56))
	for _, id := range []string{"old-first", "old-second"} {
		c := connectorProposalConnection(id)
		_, err := s.db.Exec(`INSERT INTO connections(id,owner_id,name,provider,kind,recipe_json,encrypted_secret) VALUES(?,?,?,?,?,?,?)`, c.ID, c.OwnerID, c.Name, c.Provider, c.Kind, c.RecipeJSON, c.EncryptedSecret)
		require.NoError(t, err)
	}
	require.NoError(t, s.Close())
	upgraded, err := OpenSQLite(path)
	require.NoError(t, err, "existing duplicates must not make the upgrade fail")
	defer upgraded.Close()
	connections, err := upgraded.ListConnections(context.Background(), alice)
	require.NoError(t, err)
	assert.Len(t, connections, 2, "migration must preserve existing accounts and grants")
	p := connectorProposalFixture(t, upgraded, "app")
	assert.Equal(t, "approved", p.State)
	assert.Equal(t, "old-first", p.ConnectionID)
}
