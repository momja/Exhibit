package store

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The connector branch initially used 043 before main shipped agent grants at
// that version. Upgrading either deployed baseline must apply the whole connector
// sequence without replacing the shipped grant migration or losing library data.
func TestConnectorUpgradeFromShippedMain(t *testing.T) {
	for _, version := range []int64{42, 43} {
		t.Run(fmt.Sprint(version), func(t *testing.T) {
			registerOriginNormalizationMigration()
			path := newMigratedTo(t, version)
			db, err := sql.Open("sqlite", path)
			require.NoError(t, err)
			legacy := &SQLiteStore{db: db}
			ctx := t.Context()
			require.Equal(t, version, currentVersion(t, legacy))
			require.NoError(t, legacy.PutArtifact(ctx, &Artifact{ID: "existing", OwnerID: 1, Title: "Saved tool", SourceBlobID: "body", Tier: Tier1}))
			require.NoError(t, legacy.SetState(ctx, 1, "existing", 1, "saved", "existing result"))
			var grantID string
			if version == 43 {
				grant, err := legacy.CreateAgentGrant(ctx, 1, "Existing agent", "test", "existing-digest")
				require.NoError(t, err)
				grantID = grant.ID
			}
			require.NoError(t, db.Close())

			s, err := OpenSQLite(path)
			require.NoError(t, err)
			t.Cleanup(func() { s.Close() })
			assert.Equal(t, headVersion(t), currentVersion(t, s))
			state, err := s.GetState(ctx, 1, "existing", 1)
			require.NoError(t, err)
			assert.Equal(t, "existing result", state["saved"])
			if version == 43 {
				grant, err := s.ResolveAgentGrant(ctx, "existing-digest")
				require.NoError(t, err)
				require.NotNil(t, grant)
				assert.Equal(t, grantID, grant.ID)
				assert.EqualValues(t, 1, grant.OwnerID)
			} else {
				_, err := s.CreateAgentGrant(ctx, 1, "New agent", "test", "new-digest")
				require.NoError(t, err, "upgrading from 042 also applies main's agent-grant migration")
			}

			require.NoError(t, s.CreateConnection(ctx, &ConnectionRecord{ID: "new-connection", OwnerID: 1, Name: "Account", Provider: "bearer", Kind: "native", RecipeJSON: `{"origins":["https://api.example.test"]}`, EncryptedSecret: "sealed"}, -1))
			require.NoError(t, s.PutConnectionGrant(ctx, ConnectionGrantRecord{ArtifactID: "existing", ConnectionID: "new-connection", ViewerID: 1}))
			connection, err := s.GetGrantedConnection(ctx, 1, "existing", "new-connection")
			require.NoError(t, err)
			assert.Equal(t, "sealed", connection.EncryptedSecret)
			now := time.Now()
			require.NoError(t, s.PutConnectionCache(ctx, connection.ID, "request", "cached-result", now, now.Add(time.Minute)))
			cached, err := s.GetConnectionCache(ctx, connection.ID, "request", now)
			require.NoError(t, err)
			assert.Equal(t, "cached-result", cached, "the final cache accounting migration also runs")
		})
	}
}
