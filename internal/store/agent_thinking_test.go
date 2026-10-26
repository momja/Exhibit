package store

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentThinkingPreferenceSurvivesUpgradeAndRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.db")
	s, err := OpenSQLite(path)
	require.NoError(t, err)
	require.NoError(t, s.SetAgentKey(t.Context(), &AgentKey{
		OwnerID: 1, Provider: "anthropic", Model: "model", KeyCiphertext: "sealed-key",
	}))
	// Recreate the previous release's agent_keys and ledger, with an existing
	// credential that the migration must preserve and default to inheritance.
	_, err = s.db.ExecContext(t.Context(), "ALTER TABLE agent_keys DROP COLUMN thinking_level")
	require.NoError(t, err)
	_, err = s.db.ExecContext(t.Context(), "DELETE FROM goose_db_version WHERE version_id >= 37")
	require.NoError(t, err)
	require.NoError(t, s.Close())

	s, err = OpenSQLite(path)
	require.NoError(t, err)
	k, err := s.GetAgentKey(t.Context(), 1)
	require.NoError(t, err)
	require.NotNil(t, k)
	assert.Empty(t, k.ThinkingLevel)
	assert.Equal(t, "sealed-key", k.KeyCiphertext)
	k.ThinkingLevel = "high"
	require.NoError(t, s.SetAgentKey(t.Context(), k))
	require.NoError(t, s.Close())

	s, err = OpenSQLite(path)
	require.NoError(t, err)
	defer s.Close()
	k, err = s.GetAgentKey(t.Context(), 1)
	require.NoError(t, err)
	require.NotNil(t, k)
	assert.Equal(t, "high", k.ThinkingLevel)
	assert.Equal(t, "sealed-key", k.KeyCiphertext)
	other, err := s.GetAgentKey(t.Context(), 2)
	require.NoError(t, err)
	assert.Nil(t, other, "another owner cannot read this preference or key")
}
