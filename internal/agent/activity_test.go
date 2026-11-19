package agent

import (
	"testing"

	"github.com/momja/Exhibit/internal/agentscope"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunningSessionsArePersonalAndExcludeIdleOrClosed(t *testing.T) {
	registry := agentscope.NewRegistry()
	manager := &Manager{sessions: map[string]*Session{}}
	for _, item := range []struct {
		id                string
		owner             int64
		streaming, closed bool
	}{{"running", 1, true, false}, {"idle", 1, false, false}, {"closed", 1, true, true}, {"foreign", 2, true, false}} {
		grant, err := registry.Issue(item.owner, "artifact-"+item.id, item.id)
		require.NoError(t, err)
		manager.sessions[item.id] = &Session{ID: item.id, OwnerID: item.owner, grant: grant, streaming: item.streaming, closed: item.closed, title: "Change the chart"}
	}
	running := manager.RunningSessions(1)
	require.Len(t, running, 1)
	assert.Equal(t, "running", running[0].ID)
	assert.Equal(t, "artifact-running", running[0].ArtifactID)
	assert.Equal(t, "Change the chart", running[0].Title)
	assert.Empty(t, manager.RunningSessions(0))
}
