package agent

import (
	"encoding/json"
	"testing"

	"github.com/momja/Exhibit/internal/agentscope"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newDetachedSession builds a Session with no subprocess behind it — enough to
// drive handleLine, which only touches the fanout state. artifactID sets the
// grant's bound scope, matching what the API's create handler would have
// already done before any tool_execution_end line reaches this session
// (av-e0yj); "" leaves it unbound, as a create-mode session starts.
func newDetachedSession(t *testing.T, artifactID string) *Session {
	t.Helper()
	grant, err := agentscope.NewRegistry().Issue(1, artifactID)
	require.NoError(t, err)
	return &Session{
		ID:      "test-session",
		grant:   grant,
		subs:    map[chan []byte]struct{}{},
		pending: map[string]chan json.RawMessage{},
		done:    make(chan struct{}),
	}
}

// feed pushes one pi output line through the session and returns everything it
// broadcast in response.
func feed(t *testing.T, s *Session, line string) []map[string]any {
	t.Helper()
	events, unsubscribe := s.Subscribe()
	defer unsubscribe()

	s.handleLine([]byte(line))

	var out []map[string]any
	for {
		select {
		case raw := <-events:
			var ev map[string]any
			require.NoError(t, json.Unmarshal(raw, &ev))
			out = append(out, ev)
		default:
			return out
		}
	}
}

// find returns the first broadcast event of the given type, or nil.
func find(events []map[string]any, typ string) map[string]any {
	for _, ev := range events {
		if ev["type"] == typ {
			return ev
		}
	}
	return nil
}

// A save on a session already bound to an artifact (via its grant, the way
// the API's create handler binds it — av-e0yj) broadcasts the synthetic event
// the chat page turns into a preview swap, with the id read from the grant
// rather than trusted from the tool's own reported details.
func TestSaveWithArtifactIDBroadcastsTheSavedEvent(t *testing.T) {
	s := newDetachedSession(t, "art-1")

	events := feed(t, s, `{"type":"tool_execution_end","toolName":"update_artifact","isError":false,`+
		`"result":{"details":{"exhibit":"artifact_saved","action":"updated","artifactId":"art-1",`+
		`"title":"Charty"}}}`)

	saved := find(events, "exhibit_artifact_saved")
	require.NotNil(t, saved, "a save on a bound session must broadcast exhibit_artifact_saved")
	assert.Equal(t, "art-1", saved["artifactId"])
	assert.Equal(t, "updated", saved["action"])
	assert.Equal(t, "Charty", saved["title"])
	assert.Equal(t, "art-1", s.ArtifactID(), "the session stays bound to its grant's artifact")
}

// ...and the failure mode that made av-l31x silent: a save on a session whose
// grant is still unbound yields no event at all, so the preview pane is never
// told to refresh. Nothing logs, nothing errors — the save simply goes
// unreported.
func TestSaveWithoutArtifactIDBroadcastsNothingSynthetic(t *testing.T) {
	s := newDetachedSession(t, "")

	events := feed(t, s, `{"type":"tool_execution_end","toolName":"update_artifact","isError":false,`+
		`"result":{"details":{"exhibit":"artifact_saved","action":"updated"}}}`)

	assert.Nil(t, find(events, "exhibit_artifact_saved"),
		"an unbound session must not produce a saved event — this is why the update path went unreported")
	assert.Len(t, events, 1, "only the raw pi line is forwarded")
	assert.Empty(t, s.ArtifactID())
}
