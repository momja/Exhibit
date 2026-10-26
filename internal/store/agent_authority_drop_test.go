package store

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A database the previous release left at version 37 still carries 035's
// worker-authority tables; upgrading removes them and keeps everything else.
func TestUpgradeDropsDescopedAgentAuthorityTables(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.db")
	s, err := OpenSQLite(path)
	require.NoError(t, err)
	require.NoError(t, s.SetAgentKey(t.Context(), &AgentKey{
		OwnerID: 1, Provider: "anthropic", Model: "model", KeyCiphertext: "sealed-key",
	}))
	up, err := migrationsFS.ReadFile("migrations/035_agent_authority.sql")
	require.NoError(t, err)
	upOnly, _, found := strings.Cut(string(up), "-- +goose Down")
	require.True(t, found)
	_, err = s.db.ExecContext(t.Context(), upOnly)
	require.NoError(t, err)
	_, err = s.db.ExecContext(t.Context(), "DELETE FROM goose_db_version WHERE version_id = 38")
	require.NoError(t, err)
	require.NoError(t, s.Close())

	s, err = OpenSQLite(path)
	require.NoError(t, err)
	defer s.Close()
	for _, table := range []string{"agent_conversations", "agent_executions", "agent_supervisor_credentials", "agent_usage_receipts"} {
		var n int
		require.NoError(t, s.db.QueryRowContext(t.Context(),
			"SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?", table).Scan(&n))
		assert.Zero(t, n, table)
	}
	k, err := s.GetAgentKey(t.Context(), 1)
	require.NoError(t, err)
	require.NotNil(t, k)
	assert.Equal(t, "sealed-key", k.KeyCiphertext)
}
