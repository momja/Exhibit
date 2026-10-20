package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/momja/Exhibit/internal/agentcontrol"
	"github.com/momja/Exhibit/internal/secrets"
	"github.com/momja/Exhibit/internal/sprites"
	"github.com/stretchr/testify/require"
)

func TestWorkerJournalAuthenticatesIdentityAndEncryptsEndpointToken(t *testing.T) {
	directory := t.TempDir()
	box, err := secrets.Load("journal-test-secret", "")
	require.NoError(t, err)
	journal, err := openWorkerJournal(directory, box)
	require.NoError(t, err)
	record := workerRecord{OwnerID: 19, Assignment: agentcontrol.Assignment{Identity: agentcontrol.Identity{OwnerID: 19, ConversationID: "private-conversation", ExecutionID: "attempt", WorkerID: "worker", Generation: 1}}, Request: sprites.Request{Identity: sprites.Identity{OwnerID: "19", SessionID: "private-conversation", ExecutionID: "attempt"}, EndpointToken: strings.Repeat("secret-endpoint-credential", 3)}}
	require.NoError(t, journal.update("attempt", &record))
	data, err := os.ReadFile(journal.path)
	require.NoError(t, err)
	require.NotContains(t, string(data), record.Request.EndpointToken)
	require.NotContains(t, string(data), "private-conversation")
	for path, mode := range map[string]os.FileMode{journal.path: 0600, filepath.Dir(journal.path): 0700} {
		info, err := os.Stat(path)
		require.NoError(t, err)
		require.Equal(t, mode, info.Mode().Perm())
	}
	reopened, err := openWorkerJournal(directory, box)
	require.NoError(t, err)
	require.Equal(t, record, reopened.snapshot()["attempt"])
	otherBox, err := secrets.Load("other-secret", "")
	require.NoError(t, err)
	_, err = openWorkerJournal(directory, otherBox)
	require.Error(t, err)
	require.NoError(t, journal.update("attempt", nil))
	reopened, err = openWorkerJournal(directory, box)
	require.NoError(t, err)
	require.Empty(t, reopened.snapshot())
}
