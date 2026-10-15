package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/momja/Exhibit/internal/agent"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A conversation is kept as Pi recorded it, tied to the version the artifact is
// at when the turn settles (av-y7td) — run against a real `pi` sidecar, because
// what is kept is Pi's own file and nothing else is evidence that it is.

// The recolor the mock LLM scripts replaces a background up to the next
// semicolon, so a body without one would have its tail eaten.
const counterBody = `<html><head><style>#submit-btn{background:#f7d51d;color:#333}</style></head><body><button id="submit-btn">Count!</button></body></html>`

// settles waits for n more agent_settled events on a session's event stream.
func settles(t *testing.T, events <-chan []byte, n int) {
	t.Helper()
	deadline := time.After(90 * time.Second)
	for seen := 0; seen < n; {
		select {
		case raw := <-events:
			var ev struct {
				Type string `json:"type"`
			}
			if json.Unmarshal(raw, &ev) == nil && ev.Type == "agent_settled" {
				seen++
			}
		case <-deadline:
			t.Fatalf("the agent settled %d time(s), wanted %d", seen, n)
		}
	}
}

// keptConversation waits for the session's conversation to be stored with at
// least wantUserMessages of the user's turns in it, and returns it.
// Persistence runs off the settled turn in its own goroutine.
func keptConversation(t *testing.T, r *Router, artifactID, session string, wantUserMessages int) *store.Transcript {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		tr, err := r.cfg.Store.GetTranscript(context.Background(), defaultOwnerID, artifactID, session)
		require.NoError(t, err)
		if tr != nil {
			users := 0
			for _, m := range agent.Messages(tr) {
				if m.Role == "user" {
					users++
				}
			}
			if users >= wantUserMessages {
				return tr
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("no conversation with %d user message(s) was kept", wantUserMessages)
	return nil
}

func TestAConversationIsKeptWithTheVersionItLeftBehind(t *testing.T) {
	h := newPiHarness(t)
	r := h.router
	id := createArtifact(t, r, map[string]any{"title": "Counter", "body": counterBody})

	session := startSessionFor(t, r, id)
	events, unsubscribe := r.cfg.Agent.Get(defaultOwnerID, session).Subscribe()
	defer unsubscribe()

	w := doJSON(t, r, "POST", "/api/agent/sessions/"+session+"/prompt", map[string]any{"message": "make the button purple"})
	require.Equal(t, http.StatusAccepted, w.Code)
	settles(t, events, 1)

	kept := keptConversation(t, r, id, session, 1)
	assert.True(t, kept.Resumable)
	assert.Equal(t, "make the button purple", kept.Title)
	// The artifact was at version 1 when the conversation began and the agent's
	// write made version 2, which is what it left behind.
	assert.Equal(t, 2, kept.VersionSeq)

	// It is Pi's own file: a header, and the conversation as message entries.
	assert.Contains(t, kept.SessionFile, `"type":"session"`)
	// The header records the directory the session ran in, and Pi will not
	// resume a file whose directory is gone. Every session runs from the same
	// one, which exists wherever the conversation is later resumed.
	header := strings.SplitN(kept.SessionFile, "\n", 2)[0]
	assert.Contains(t, header, `"cwd":"/"`)
	assert.Contains(t, kept.SessionFile, `"role":"user"`)
	assert.Contains(t, kept.SessionFile, `"name":"get_artifact"`)

	// And what a person sees of it is what they saw live.
	msgs := agent.Messages(kept)
	require.GreaterOrEqual(t, len(msgs), 4, "%+v", msgs)
	assert.Equal(t, agent.TranscriptMessage{Role: "user", Text: "make the button purple"}, msgs[0])
	assert.Equal(t, agent.TranscriptMessage{Role: "tool", Text: "Reading artifact source"}, msgs[1])
	assert.Equal(t, agent.TranscriptMessage{Role: "tool", Text: "Writing artifact"}, msgs[2])
	assert.Equal(t, "assistant", msgs[len(msgs)-1].Role)
}

// A conversation is one record, kept in place as it goes on: another turn
// moves it to the version that turn left behind and adds to what was said.
func TestAConversationIsKeptInPlaceAsItGoesOn(t *testing.T) {
	h := newPiHarness(t)
	r := h.router
	id := createArtifact(t, r, map[string]any{"title": "Counter", "body": counterBody})

	session := startSessionFor(t, r, id)
	events, unsubscribe := r.cfg.Agent.Get(defaultOwnerID, session).Subscribe()
	defer unsubscribe()

	for _, prompt := range []string{"make the button purple", "now make it red"} {
		w := doJSON(t, r, "POST", "/api/agent/sessions/"+session+"/prompt", map[string]any{"message": prompt})
		require.Equal(t, http.StatusAccepted, w.Code)
		settles(t, events, 1)
	}

	kept := keptConversation(t, r, id, session, 2)
	assert.Equal(t, "make the button purple", kept.Title, "a conversation is named by how it began")
	assert.Equal(t, 3, kept.VersionSeq, "two agent writes on top of the first version")

	list, err := r.cfg.Store.ListTranscripts(context.Background(), defaultOwnerID, id)
	require.NoError(t, err)
	assert.Len(t, list, 1, "one conversation, not one per turn")
}

// conversation runs one settled turn on a fresh session and returns its id once
// the conversation is kept.
func conversation(t *testing.T, h *piHarness, artifactID, prompt string) string {
	t.Helper()
	r := h.router
	session := startSessionFor(t, r, artifactID)
	events, unsubscribe := r.cfg.Agent.Get(defaultOwnerID, session).Subscribe()
	defer unsubscribe()
	w := doJSON(t, r, "POST", "/api/agent/sessions/"+session+"/prompt", map[string]any{"message": prompt})
	require.Equal(t, http.StatusAccepted, w.Code)
	settles(t, events, 1)
	keptConversation(t, r, artifactID, session, 1)
	return session
}

// closeAndWait ends a session and waits until it is gone from the registry, so
// that a resume that follows is a resume and not an attach.
func closeAndWait(t *testing.T, r *Router, session string) {
	t.Helper()
	r.cfg.Agent.Close(defaultOwnerID, session)
	require.Eventually(t, func() bool { return r.cfg.Agent.Get(defaultOwnerID, session) == nil }, 5*time.Second, 20*time.Millisecond)
}

// The scratch directory is a working copy; the conversation's home is the
// database. It goes when the session ends, so a conversation is not left on
// disk after the account that owned it is erased.
func TestASessionsScratchDirectoryGoesWhenItEnds(t *testing.T) {
	h := newPiHarness(t)
	r := h.router
	id := createArtifact(t, r, map[string]any{"title": "Counter", "body": counterBody})
	session := conversation(t, h, id, "make the button purple")

	sessionDirs := func() []string {
		entries, err := os.ReadDir(h.workRoot)
		require.NoError(t, err)
		var dirs []string
		for _, e := range entries {
			if e.IsDir() {
				dirs = append(dirs, e.Name())
			}
		}
		return dirs
	}
	require.Len(t, sessionDirs(), 1, "a running session has a directory")

	closeAndWait(t, r, session)
	require.Eventually(t, func() bool { return len(sessionDirs()) == 0 }, 10*time.Second, 50*time.Millisecond,
		"nothing of the conversation is left on disk")

	// Its record in the database is untouched, and is what a resume starts from.
	assert.NotNil(t, keptConversation(t, r, id, session, 1))
}
