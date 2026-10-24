package store

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateConnectionPermissionsPreservesRecordAndRelatedData(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	const artifactID = "permission-preservation"
	require.NoError(t, s.PutArtifact(ctx, &Artifact{ID: artifactID, OwnerID: alice, Title: "Existing app", SourceBlobID: "body", Tier: Tier1}))
	require.NoError(t, s.SetStateAsViewer(ctx, ViewerID(alice), artifactID, "saved", "existing state"))
	for _, provider := range []string{"api-key", "bearer", "basic"} {
		t.Run(provider, func(t *testing.T) {
			connection := &ConnectionRecord{
				ID: "editable-" + provider, OwnerID: alice, Name: "Existing " + provider, Provider: provider, Kind: "native",
				RecipeJSON:      fmt.Sprintf(`{"origins":["https://%s.example.test"],"header":"Authorization","scheme":"Bearer ","allow_writes":false,"daily_limit":25,"cache_seconds":600,"endpoint":"/","sensitivity":"personal","future":{"large_number":9007199254740993,"enabled":true,"nested":["keep",{"base":"/private/"}]}}`, provider),
				EncryptedSecret: "original-sealed-credential-" + provider + "==",
			}
			require.NoError(t, s.CreateConnection(ctx, connection, -1))
			require.NoError(t, s.TouchConnection(ctx, connection.ID))
			require.NoError(t, s.PutConnectionGrant(ctx, ConnectionGrantRecord{ArtifactID: artifactID, ConnectionID: connection.ID, ViewerID: alice, PermittedEgress: []string{"https://legacy.example.test"}}))
			now := time.Now()
			require.NoError(t, s.ReserveConnectionRequest(ctx, connection.ID, 25, now))
			require.NoError(t, s.PutConnectionCache(ctx, connection.ID, "existing-response", "original-encrypted-response", now.Add(time.Hour)))
			before, err := s.GetConnection(ctx, alice, connection.ID)
			require.NoError(t, err)
			grants, err := s.ListConnectionGrants(ctx, alice, artifactID)
			require.NoError(t, err)
			provenance, err := s.ListConnectionDataPolicies(ctx, alice, artifactID)
			require.NoError(t, err)
			var originalRecipe map[string]json.RawMessage
			require.NoError(t, json.Unmarshal([]byte(before.RecipeJSON), &originalRecipe))
			delete(originalRecipe, "allow_writes")
			for _, permission := range []bool{true, true, false, false} {
				require.NoError(t, s.UpdateConnectionPermissions(ctx, alice, connection.ID, permission))
				after, err := s.GetConnection(ctx, alice, connection.ID)
				require.NoError(t, err)
				var updatedRecipe map[string]json.RawMessage
				require.NoError(t, json.Unmarshal([]byte(after.RecipeJSON), &updatedRecipe))
				assert.Equal(t, fmt.Sprint(permission), string(updatedRecipe["allow_writes"]))
				delete(updatedRecipe, "allow_writes")
				assert.Equal(t, originalRecipe, updatedRecipe, "known and unknown recipe fields must survive without re-encoding their values")
				var jsonType string
				require.NoError(t, s.db.QueryRowContext(ctx, `SELECT json_type(recipe_json,'$.allow_writes') FROM connections WHERE id=?`, connection.ID).Scan(&jsonType))
				assert.Equal(t, fmt.Sprint(permission), jsonType, "the stored permission is a JSON boolean")
				beforeFields, afterFields := *before, *after
				beforeFields.RecipeJSON, afterFields.RecipeJSON = "", ""
				assert.Equal(t, beforeFields, afterFields, "the secret, timestamps and every other record column must remain exact")
			}
			afterGrants, err := s.ListConnectionGrants(ctx, alice, artifactID)
			require.NoError(t, err)
			assert.Equal(t, grants, afterGrants)
			afterProvenance, err := s.ListConnectionDataPolicies(ctx, alice, artifactID)
			require.NoError(t, err)
			assert.Equal(t, provenance, afterProvenance)
			requests, err := s.ConnectionRequestsToday(ctx, connection.ID, now)
			require.NoError(t, err)
			assert.Equal(t, 1, requests)
			cached, err := s.GetConnectionCache(ctx, connection.ID, "existing-response", now)
			require.NoError(t, err)
			assert.Equal(t, "original-encrypted-response", cached)
			state, err := s.GetStateAsViewer(ctx, ViewerID(alice), artifactID)
			require.NoError(t, err)
			assert.Equal(t, map[string]string{"saved": "existing state"}, state)
		})
	}
}

func TestUpdateConnectionPermissionsRequiresOwnerAndActiveNativeProvider(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	for _, tc := range []struct{ id, provider, kind, status string }{
		{"foreign", "bearer", "native", "active"},
		{"revoked", "bearer", "native", "revoked"},
		{"reconnect", "basic", "native", "reconnect"},
		{"oauth", "github", "oauth", "active"},
		{"vendor", "bearer", "vendor", "active"},
		{"unsupported-native", "custom", "native", "active"},
	} {
		t.Run(tc.id, func(t *testing.T) {
			record := &ConnectionRecord{ID: tc.id, OwnerID: alice, Name: tc.id, Provider: tc.provider, Kind: tc.kind, RecipeJSON: `{"allow_writes":false,"preserve":"unchanged"}`, EncryptedSecret: "do-not-replace"}
			require.NoError(t, s.CreateConnection(ctx, record, -1))
			if tc.status != "active" {
				_, err := s.db.ExecContext(ctx, `UPDATE connections SET status=? WHERE id=?`, tc.status, tc.id)
				require.NoError(t, err)
			}
			before, err := s.GetConnection(ctx, alice, tc.id)
			require.NoError(t, err)
			owner := alice
			if tc.id == "foreign" {
				owner = bob
			}
			assert.ErrorIs(t, s.UpdateConnectionPermissions(ctx, owner, tc.id, true), ErrNotFound)
			after, err := s.GetConnection(ctx, alice, tc.id)
			require.NoError(t, err)
			assert.Equal(t, before, after)
		})
	}
	assert.ErrorIs(t, s.UpdateConnectionPermissions(ctx, alice, "missing", true), ErrNotFound)
	require.NoError(t, s.CreateConnection(ctx, &ConnectionRecord{ID: "deleted", OwnerID: alice, Name: "Deleted", Provider: "bearer", Kind: "native", RecipeJSON: `{}`, EncryptedSecret: "sealed"}, -1))
	require.NoError(t, s.DeleteConnection(ctx, alice, "deleted"))
	assert.ErrorIs(t, s.UpdateConnectionPermissions(ctx, alice, "deleted", true), ErrNotFound)
}
