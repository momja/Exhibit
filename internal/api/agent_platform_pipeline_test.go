package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A whole agent turn in platform mode, in process, against a real
// `pi --mode rpc` sidecar (av-siqf).
//
// This is the test the ticket asked for by name. "The user never learns what
// is under the hood" is a claim about Pi's event stream and about the
// transcript that stores it, not about one JSON route — and Pi's protocol puts
// api/provider/model on every assistant message it emits. Both seams are
// unfiltered passthroughs by construction, so the only way to keep this true
// as Pi's protocol moves is to run a turn and look at what came out.

// TestPlatformModeRunsASessionWithNoStoredKey is the mode's whole reason to
// exist: an owner who has never entered a key gets a working agent.
func TestPlatformModeRunsASessionWithNoStoredKey(t *testing.T) {
	h := newPlatformPiHarness(t)
	r := h.router

	// The premise, stated rather than assumed.
	k, err := r.cfg.Store.GetAgentKey(context.Background(), defaultOwnerID)
	require.NoError(t, err)
	require.Nil(t, k, "this owner must have no stored key, or the test proves nothing")

	id := createArtifact(t, r, map[string]any{
		"title": "Counter",
		"body":  `<html><head><style>#submit-btn{background:#f7d51d}</style></head><body><button id="submit-btn">Count!</button></body></html>`,
	})

	session := startSessionFor(t, r, id)
	w := doJSON(t, r, "POST", "/api/agent/sessions/"+session+"/prompt",
		map[string]any{"message": "make the button green"})
	require.Equal(t, http.StatusAccepted, w.Code)

	waitForBody(t, r, id, func(b string) bool {
		return strings.Contains(b, "#22a15c")
	}, "the agent to run on the instance's own credential")
}

// The create-session response is the other thing the browser sees, and it
// named the provider and the model before this ticket. Nothing on the page
// reads either field, so platform mode simply omits them.
func TestPlatformModeSessionResponseNamesNoModel(t *testing.T) {
	h := newPlatformPiHarness(t)
	r := h.router

	w := doJSON(t, r, "POST", "/api/agent/sessions", map[string]string{})
	require.Equal(t, http.StatusCreated, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Contains(t, resp, "id")
	assert.NotContains(t, resp, "provider")
	assert.NotContains(t, resp, "model")
	assert.NotContains(t, w.Body.String(), "exhibit-mock")
	r.cfg.Agent.Close(defaultOwnerID, resp["id"].(string))
}

// BYOK still reports them: the caller configured that key, and this is their
// own choice described back to them.
func TestBYOKSessionResponseStillNamesTheModel(t *testing.T) {
	h := newPiHarness(t)
	r := h.router

	w := doJSON(t, r, "POST", "/api/agent/sessions", map[string]string{})
	require.Equal(t, http.StatusCreated, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "exhibit-mock", resp["provider"])
	assert.Equal(t, "exhibit-mock-1", resp["model"])
	r.cfg.Agent.Close(defaultOwnerID, resp["id"].(string))
}

// The stream and the transcript are where the abstraction actually holds or
// fails, so both are read back and searched for the identifiers Pi emits.
func TestPlatformModeStreamAndTranscriptNameNoModel(t *testing.T) {
	h := newPlatformPiHarness(t)
	r := h.router

	id := createArtifact(t, r, map[string]any{
		"title": "Counter",
		"body":  `<html><head><style>#submit-btn{background:#f7d51d}</style></head><body><button id="submit-btn">Count!</button></body></html>`,
	})

	session := startSessionFor(t, r, id)
	s := r.cfg.Agent.Get(defaultOwnerID, session)
	require.NotNil(t, s)
	events, unsubscribe := s.Subscribe()
	defer unsubscribe()

	var streamed []string
	collected := make(chan struct{})
	go func() {
		defer close(collected)
		for {
			select {
			case line := <-events:
				streamed = append(streamed, string(line))
			case <-time.After(8 * time.Second):
				return
			}
		}
	}()

	w := doJSON(t, r, "POST", "/api/agent/sessions/"+session+"/prompt",
		map[string]any{"message": "make the button green"})
	require.Equal(t, http.StatusAccepted, w.Code)
	waitForBody(t, r, id, func(b string) bool {
		return strings.Contains(b, "#22a15c")
	}, "the agent to finish a turn")
	<-collected

	require.NotEmpty(t, streamed, "no events were broadcast, so nothing was checked")
	stream := strings.Join(streamed, "\n")
	// The provider name, the model name, and the field names that would carry
	// either one on a message envelope.
	for _, leak := range []string{"exhibit-mock", `"provider"`, `"model"`, `"api"`} {
		assert.NotContains(t, stream, leak, "the SSE passthrough must not name the model")
	}
	// The turn still streamed — this is not passing because nothing happened.
	assert.Contains(t, stream, "agent_settled")
	assert.Contains(t, stream, "exhibit_artifact_saved")
	// And it is still Pi's protocol, minus three fields.
	assert.Contains(t, stream, `"role":"assistant"`)
	assert.Contains(t, stream, `"usage"`, "token usage is deliberately kept for metering")

	transcripts := waitForTranscript(t, r, id)
	for _, leak := range []string{"exhibit-mock", `"provider"`, `"model"`, `"api"`} {
		assert.NotContains(t, transcripts, leak, "the persisted transcript must not name the model")
	}
}

// BYOK is the control: the same rig without platform mode still reports what
// the owner's own key is, which is what makes the assertions above a test of
// the filter rather than of Pi having stopped emitting anything.
func TestBYOKStreamStillNamesTheOwnersModel(t *testing.T) {
	h := newPiHarness(t)
	r := h.router

	id := createArtifact(t, r, map[string]any{
		"title": "Counter",
		"body":  `<html><head><style>#submit-btn{background:#f7d51d}</style></head><body><button id="submit-btn">Count!</button></body></html>`,
	})

	session := startSessionFor(t, r, id)
	s := r.cfg.Agent.Get(defaultOwnerID, session)
	require.NotNil(t, s)
	events, unsubscribe := s.Subscribe()
	defer unsubscribe()

	var streamed []string
	collected := make(chan struct{})
	go func() {
		defer close(collected)
		for {
			select {
			case line := <-events:
				streamed = append(streamed, string(line))
			case <-time.After(8 * time.Second):
				return
			}
		}
	}()

	w := doJSON(t, r, "POST", "/api/agent/sessions/"+session+"/prompt",
		map[string]any{"message": "make the button green"})
	require.Equal(t, http.StatusAccepted, w.Code)
	waitForBody(t, r, id, func(b string) bool {
		return strings.Contains(b, "#22a15c")
	}, "the agent to finish a turn")
	<-collected

	assert.Contains(t, strings.Join(streamed, "\n"), "exhibit-mock",
		"BYOK must be unchanged: Pi still reports the key the owner entered")
}

// startSessionFor opens a modify session bound to an artifact and returns its id.
func startSessionFor(t *testing.T, r *Router, artifactID string) string {
	t.Helper()
	w := doJSON(t, r, "POST", "/api/agent/sessions", map[string]string{"artifact_id": artifactID})
	require.Equal(t, http.StatusCreated, w.Code)
	var session struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &session))
	t.Cleanup(func() { r.cfg.Agent.Close(defaultOwnerID, session.ID) })
	return session.ID
}

// waitForTranscript reads an artifact's persisted agent transcripts back
// through the API, as the colophon UI does, once one holding an assistant
// message has landed. Persistence runs off the settled turn in its own
// goroutine, and an empty list would satisfy every "does not contain"
// assertion for the wrong reason.
func waitForTranscript(t *testing.T, r *Router, artifactID string) string {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	var body string
	for time.Now().Before(deadline) {
		w := doJSON(t, r, "GET", "/api/artifacts/"+artifactID+"/transcripts", nil)
		require.Equal(t, http.StatusOK, w.Code)
		body = w.Body.String()
		if strings.Contains(body, "assistant") {
			return body
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("no transcript with an assistant message was persisted; last response:\n%s", body)
	return ""
}
