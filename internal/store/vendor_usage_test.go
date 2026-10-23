package store

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVendorConnectionEngineAndExternalAccountStayPinned(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	connection := &ConnectionRecord{ID: "managed", OwnerID: alice, Name: "Work account", Provider: "slack", Kind: "vendor", Engine: "composio", ExternalRef: "external-account-private", RecipeJSON: `{"origins":["https://slack.com"]}`, EncryptedSecret: "encrypted-private"}
	require.NoError(t, s.CreateConnection(ctx, connection, -1))
	stored, err := s.GetConnection(ctx, alice, connection.ID)
	require.NoError(t, err)
	assert.Equal(t, "composio", stored.Engine)
	assert.Equal(t, "external-account-private", stored.ExternalRef)
	assert.ErrorIs(t, s.UpdateVendorConnectionRef(ctx, connection.ID, "nango", "different-account"), ErrNotFound)
	assert.ErrorIs(t, s.UpdateVendorConnectionRef(ctx, connection.ID, "composio", "different-account"), ErrNotFound)
	require.NoError(t, s.UpdateVendorConnectionRef(ctx, connection.ID, "composio", stored.ExternalRef))
	raw, err := json.Marshal(stored)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "external-account-private")
	assert.NotContains(t, string(raw), "encrypted-private")
	assert.NotContains(t, string(raw), "composio")
	_, err = s.GetConnection(ctx, bob, connection.ID)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestVendorEngineMigrationPinsOriginalEngineAndLazilyAddsExternalRef(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	require.NoError(t, goose.DownToContext(ctx, s.db, "migrations", 47))
	_, err := s.db.ExecContext(ctx, `INSERT INTO connections(id,owner_id,name,provider,kind,recipe_json,encrypted_secret) VALUES
 ('legacy',?,'Legacy account','nango','vendor','{"vendor_provider":"slack","origins":["https://slack.com"]}','old-encrypted-id'),
 ('native',?,'Personal account','basic','native','{}','native-encrypted'),
 ('malformed',?,'Legacy invalid','nango','vendor','invalid-old-policy','sealed')`, alice, alice, alice)
	require.NoError(t, err)
	require.NoError(t, s.migrate())
	legacy, err := s.GetConnection(ctx, alice, "legacy")
	require.NoError(t, err)
	assert.Equal(t, "nango", legacy.Engine)
	assert.Equal(t, "slack", legacy.Provider)
	assert.Empty(t, legacy.ExternalRef)
	assert.Equal(t, "old-encrypted-id", legacy.EncryptedSecret)
	require.NoError(t, s.UpdateVendorConnectionRef(ctx, "legacy", "nango", "recovered-external-id"))
	legacy, err = s.GetConnection(ctx, alice, "legacy")
	require.NoError(t, err)
	assert.Equal(t, "recovered-external-id", legacy.ExternalRef)
	native, err := s.GetConnection(ctx, alice, "native")
	require.NoError(t, err)
	assert.Empty(t, native.Engine)
	assert.Empty(t, native.ExternalRef)
	malformed, err := s.GetConnection(ctx, alice, "malformed")
	require.NoError(t, err)
	assert.Equal(t, "nango", malformed.Engine)
}

func TestVendorUsageTotalsCurrentUTCMonthAndCurrentStoredConnections(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	for _, connection := range []*ConnectionRecord{
		{ID: "nango-slack", OwnerID: alice, Name: "Old account", Provider: "slack", Kind: "vendor", Engine: "nango", RecipeJSON: "{}", EncryptedSecret: "sealed"},
		{ID: "composio-slack", OwnerID: bob, Name: "New account", Provider: "slack", Kind: "vendor", Engine: "composio", RecipeJSON: "{}", EncryptedSecret: "sealed"},
		{ID: "native-slack", OwnerID: bob, Name: "Native", Provider: "slack", Kind: "native", RecipeJSON: "{}", EncryptedSecret: "sealed"},
	} {
		require.NoError(t, s.CreateConnection(ctx, connection, -1))
	}
	// Last month's and future calls do not enter the current billing period.
	require.NoError(t, s.RecordVendorCall(ctx, "composio", "slack", "forward", time.Date(2026, 10, 1, 0, 30, 0, 0, time.FixedZone("east", 3600))))
	require.NoError(t, s.RecordVendorCall(ctx, "composio", "slack", "forward", now.AddDate(0, 1, 0)))
	require.NoError(t, s.RecordVendorCall(ctx, "composio", "slack", "forward", now.AddDate(0, 0, 1)))
	require.NoError(t, s.RecordVendorCall(ctx, "composio", "slack", "management", now.AddDate(0, 0, -1)))
	require.NoError(t, s.RecordVendorCall(ctx, "composio", "slack", "forward", now))
	require.NoError(t, s.RecordVendorCall(ctx, "nango", "slack", "inject", now))
	usage, err := s.VendorUsage(ctx, now)
	require.NoError(t, err)
	assert.Equal(t, []VendorUsage{{Engine: "composio", Provider: "slack", StoredConnections: 1, ManagementCalls: 1, ForwardCalls: 1}, {Engine: "nango", Provider: "slack", StoredConnections: 1, InjectCalls: 1}}, usage)
	require.NoError(t, s.DeleteConnection(ctx, bob, "composio-slack"))
	usage, err = s.VendorUsage(ctx, now)
	require.NoError(t, err)
	assert.Zero(t, usage[0].StoredConnections)
	assert.EqualValues(t, 1, usage[0].ForwardCalls, "deleting an account must not erase anonymous spent usage")
	nextMonth, err := s.VendorUsage(ctx, now.AddDate(0, 1, 0))
	require.NoError(t, err)
	assert.EqualValues(t, 1, nextMonth[0].ForwardCalls)
	assert.Zero(t, nextMonth[0].ManagementCalls)
}

func TestVendorUsageConcurrentCallsAreAtomicAndOnlyAggregateSafeFields(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	var wg sync.WaitGroup
	for range 40 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			assert.NoError(t, s.RecordVendorCall(ctx, "nango", "slack", "management", now))
		}()
	}
	wg.Wait()
	usage, err := s.VendorUsage(ctx, now)
	require.NoError(t, err)
	require.Len(t, usage, 1)
	assert.EqualValues(t, 40, usage[0].ManagementCalls)
	for _, invalid := range []struct{ engine, provider, operation string }{{"unknown", "slack", "management"}, {"composio", "https://user:secret@api.example/path", "forward"}, {"nango", "slack", "response-body-private"}} {
		err := s.RecordVendorCall(ctx, invalid.engine, invalid.provider, invalid.operation, now)
		require.Error(t, err)
		assert.NotContains(t, err.Error(), "secret")
		assert.NotContains(t, err.Error(), "response-body-private")
	}
	rows, err := s.db.QueryContext(ctx, `PRAGMA table_info(vendor_usage)`)
	require.NoError(t, err)
	defer rows.Close()
	var columns []string
	for rows.Next() {
		var cid, notnull, pk int
		var name, kind string
		var defaultValue any
		require.NoError(t, rows.Scan(&cid, &name, &kind, &notnull, &defaultValue, &pk))
		columns = append(columns, name)
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, []string{"engine", "provider", "day", "management_calls", "forward_calls", "inject_calls"}, columns)
}

func TestVendorReconnectStatusPreservesExistingGrantAndRejectsNewGrants(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	connection := &ConnectionRecord{ID: "reconnect", OwnerID: alice, Name: "Work account", Provider: "slack", Kind: "vendor", Engine: "nango", ExternalRef: "external", RecipeJSON: `{"origins":["https://slack.com"]}`, EncryptedSecret: "sealed"}
	require.NoError(t, s.CreateConnection(ctx, connection, -1))
	for _, id := range []string{"existing", "new"} {
		require.NoError(t, s.PutArtifact(ctx, &Artifact{ID: id, OwnerID: alice, Title: id, SourceBlobID: "body", Tier: Tier1}))
	}
	require.NoError(t, s.PutConnectionGrant(ctx, ConnectionGrantRecord{ArtifactID: "existing", ConnectionID: connection.ID, ViewerID: alice, ShareWithRecipients: true}))
	assert.ErrorIs(t, s.MarkVendorConnectionReconnect(ctx, bob, connection.ID), ErrNotFound)
	require.NoError(t, s.MarkVendorConnectionReconnect(ctx, alice, connection.ID))
	require.NoError(t, s.MarkVendorConnectionReconnect(ctx, alice, connection.ID))
	assert.ErrorIs(t, s.CreateConnection(ctx, &ConnectionRecord{ID: "over-limit", OwnerID: alice, Name: "New", Provider: "slack", Kind: "vendor", Engine: "composio", RecipeJSON: "{}", EncryptedSecret: "sealed"}, 1), ErrConnectionLimit,
		"a stored account needing reconnect still consumes vendor capacity")
	for _, viewer := range []int64{alice, bob} {
		grants, err := s.ListConnectionGrants(ctx, viewer, "existing")
		require.NoError(t, err)
		require.Len(t, grants, 1)
		granted, err := s.GetGrantedConnection(ctx, viewer, "existing", connection.ID)
		require.NoError(t, err)
		assert.Equal(t, "reconnect", granted.Status)
	}
	assert.ErrorIs(t, s.PutConnectionGrant(ctx, ConnectionGrantRecord{ArtifactID: "new", ConnectionID: connection.ID, ViewerID: alice}), ErrNotFound)
	_, err := s.db.ExecContext(ctx, `UPDATE connections SET status='revoked' WHERE id=?`, connection.ID)
	require.NoError(t, err)
	assert.ErrorIs(t, s.MarkVendorConnectionReconnect(ctx, alice, connection.ID), ErrNotFound)
	_, err = s.GetGrantedConnection(ctx, alice, "existing", connection.ID)
	assert.ErrorIs(t, err, ErrNotFound)
	grants, err := s.ListConnectionGrants(ctx, alice, "existing")
	require.NoError(t, err)
	assert.Empty(t, grants)
}
