package agent

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newDetachedSession builds a Session with no subprocess behind it — enough to
// drive handleLine, which only touches the fanout state.
func newDetachedSession() *Session {
	return &Session{
		ID:      "test-session",
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

// A save carrying an artifact id binds the session to that artifact and
// broadcasts the synthetic event the chat page turns into a preview swap.
func TestSaveWithArtifactIDBroadcastsTheSavedEvent(t *testing.T) {
	s := newDetachedSession()

	events := feed(t, s, `{"type":"tool_execution_end","toolName":"update_artifact","isError":false,`+
		`"result":{"details":{"exhibit":"artifact_saved","action":"updated","artifactId":"art-1",`+
		`"title":"Charty"}}}`)

	saved := find(events, "exhibit_artifact_saved")
	require.NotNil(t, saved, "a save with an id must broadcast exhibit_artifact_saved")
	assert.Equal(t, "art-1", saved["artifactId"])
	assert.Equal(t, "updated", saved["action"])
	assert.Equal(t, "Charty", saved["title"])
	assert.Equal(t, "art-1", s.ArtifactID, "the session binds to the artifact it saved")
}

// ...and the failure mode that made av-l31x silent: an id the tool never
// filled in yields no event at all, so the preview pane is never told to
// refresh. Nothing logs, nothing errors — the save simply goes unreported.
func TestSaveWithoutArtifactIDBroadcastsNothingSynthetic(t *testing.T) {
	s := newDetachedSession()

	events := feed(t, s, `{"type":"tool_execution_end","toolName":"update_artifact","isError":false,`+
		`"result":{"details":{"exhibit":"artifact_saved","action":"updated"}}}`)

	assert.Nil(t, find(events, "exhibit_artifact_saved"),
		"an empty artifact id must not produce a saved event — this is why the update path went unreported")
	assert.Len(t, events, 1, "only the raw pi line is forwarded")
	assert.Empty(t, s.ArtifactID)
}
