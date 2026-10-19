package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/momja/Exhibit/internal/agentscope"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Real Pi 1.3: exercise context projection, RPC compaction, durable custom
// entries, and resume through the same materialized modules production uses.
func TestRealPiPreparedSourceLifecycle(t *testing.T) {
	pi, err := exec.LookPath("pi")
	if err != nil {
		t.Skip("pi is not installed")
	}
	m := newLimitedManager(t)
	m.cfg.PiBin = pi
	var mu sync.Mutex
	revision, source, reads := "r1", "PRIVATE-SOURCE-r1", 0
	var requests [][]map[string]any
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		grant := m.cfg.Credentials.Resolve(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		if grant == nil || grant.Scope() != (agentscope.Scope{OwnerID: 7, ArtifactID: "artifact"}) || !strings.HasPrefix(r.URL.Path, "/api/artifacts/artifact") {
			http.Error(w, "forbidden", 403)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/assets") {
			fmt.Fprint(w, `{"assets":[]}`)
			return
		}
		data := map[string]any{"id": "artifact", "title": "Title", "source_revision": revision}
		if r.URL.Query().Get("body") == "true" {
			reads++
			data["body"] = source
		}
		json.NewEncoder(w).Encode(data)
	}))
	defer api.Close()
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Messages []map[string]any `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		mu.Lock()
		requests = append(requests, request.Messages)
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"id\":\"reply\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"A concise response and compaction summary.\"},\"finish_reason\":null}]}\n\n")
		fmt.Fprint(w, "data: {\"id\":\"reply\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer provider.Close()
	m.cfg.APIBaseURL, m.cfg.MockLLMURL = api.URL, provider.URL
	require.NoError(t, m.st.PutArtifact(context.Background(), &store.Artifact{ID: "artifact", OwnerID: 7, Title: "T", SourceBlobID: "body", Tier: 1}))
	create := func(resume string) *Session {
		s, err := m.Create(context.Background(), CreateOpts{OwnerID: 7, ArtifactID: "artifact", Provider: "exhibit-mock", Model: "exhibit-mock-1", APIKey: "test", ResumeSessionID: resume})
		require.NoError(t, err)
		return s
	}
	waitRead := func(minimum int) {
		require.Eventually(t, func() bool { mu.Lock(); defer mu.Unlock(); return reads >= minimum }, 10*time.Second, 10*time.Millisecond)
	}
	send := func(s *Session, text string) {
		events, unsubscribe := s.Subscribe()
		defer unsubscribe()
		// Subscribe includes backlog; wait for this prompt's settle only.
	drain:
		for {
			select {
			case <-events:
			default:
				break drain
			}
		}
		require.NoError(t, s.Prompt(context.Background(), text, nil, nil))
		deadline := time.After(15 * time.Second)
		for {
			select {
			case event := <-events:
				if strings.Contains(string(event), `"type":"agent_settled"`) {
					return
				}
			case <-deadline:
				t.Fatal("Pi never settled")
			}
		}
	}
	sourcePair := func(messages []map[string]any, expected string) {
		found := false
		for i, message := range messages {
			raw, _ := json.Marshal(message)
			if strings.Contains(string(raw), expected) {
				assert.Equal(t, "tool", message["role"], "source is tool-role data")
				require.Greater(t, i, 0)
				call := messages[i-1]
				require.Equal(t, "assistant", call["role"])
				calls, ok := call["tool_calls"].([]any)
				require.True(t, ok)
				require.Len(t, calls, 1)
				assert.Equal(t, calls[0].(map[string]any)["id"], message["tool_call_id"])
				function := calls[0].(map[string]any)["function"].(map[string]any)
				assert.Equal(t, "get_artifact", function["name"])
				found = true
			}
		}
		assert.True(t, found, "source is present before the first provider response")
	}
	s := create("")
	waitRead(1)
	mu.Lock()
	assert.Empty(t, requests, "preparation invokes no provider")
	source, revision = "PRIVATE-SOURCE-r2", "r2"
	mu.Unlock()
	send(s, "first accepted prompt")
	mu.Lock()
	first := requests[0]
	firstReads := reads
	mu.Unlock()
	sourcePair(first, "PRIVATE-SOURCE-r2")
	assert.NotContains(t, fmt.Sprint(first), "PRIVATE-SOURCE-r1")
	assert.Equal(t, 2, firstReads, "stale warm snapshot is refreshed")
	file, err := os.ReadFile(s.sessionFile)
	require.NoError(t, err)
	assert.Contains(t, string(file), `"customType":"exhibit_source_read"`)
	assert.NotContains(t, string(file), `"role":"custom"`, "raw data is never a customMessage that becomes a user message")
	// Give Pi enough user-authored history to exceed its default retained
	// tail. No source is put in that history to manufacture compaction.
	send(s, strings.Repeat("User-authored background for the task. ", 3500))
	// Explicit RPC compaction must continue with a new read after source changes.
	response, err := s.roundTrip(context.Background(), map[string]any{"type": "compact"})
	require.NoError(t, err)
	var result struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	require.NoError(t, json.Unmarshal(response, &result))
	require.True(t, result.Success, result.Error)
	mu.Lock()
	source, revision = "PRIVATE-SOURCE-r3", "r3"
	requestCount := len(requests)
	mu.Unlock()
	send(s, "after compaction")
	mu.Lock()
	afterCompaction := requests[requestCount]
	mu.Unlock()
	sourcePair(afterCompaction, "PRIVATE-SOURCE-r3")
	// Persist through Pi, then continue both current and pre-feature session data.
	s.persistTranscript("artifact")
	id := s.ID
	m.Close(7, id)
	<-s.Done()
	waitForConversationRelease(t, m, id)
	resumed := create(id)
	send(resumed, "after resume")
	mu.Lock()
	afterResume := requests[len(requests)-1]
	mu.Unlock()
	sourcePair(afterResume, "PRIVATE-SOURCE-r3")
	resumed.persistTranscript("artifact")
	m.Close(7, id)
	<-resumed.Done()
	waitForConversationRelease(t, m, id)
	kept, err := m.st.GetTranscript(context.Background(), 7, "artifact", id)
	require.NoError(t, err)
	// This is inactive stored data, never an edit to a live Pi JSONL. Keeping
	// entry ids/parents while ignoring custom data models a pre-feature resume.
	kept.SessionFile = strings.ReplaceAll(kept.SessionFile, `"customType":"exhibit_source_read"`, `"customType":"legacy_data"`)
	require.NoError(t, m.st.SaveTranscript(context.Background(), 7, *kept))
	legacy := create(id)
	mu.Lock()
	source, revision = "PRIVATE-SOURCE-r4", "r4"
	requestCount = len(requests)
	mu.Unlock()
	send(legacy, "after old session resume")
	mu.Lock()
	afterLegacy := requests[requestCount]
	mu.Unlock()
	sourcePair(afterLegacy, "PRIVATE-SOURCE-r4")
	// All source bytes observed on every provider request retain the tool role.
	mu.Lock()
	all := append([][]map[string]any(nil), requests...)
	mu.Unlock()
	for _, request := range all {
		for _, message := range request {
			raw, _ := json.Marshal(message)
			if strings.Contains(string(raw), "PRIVATE-SOURCE-") {
				assert.Equal(t, "tool", message["role"])
			}
		}
	}
}
