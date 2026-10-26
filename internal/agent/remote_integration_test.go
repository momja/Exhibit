package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/momja/Exhibit/internal/agentexec"
	"github.com/momja/Exhibit/internal/agentscope"
	"github.com/momja/Exhibit/internal/mockllm"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Exercise the unchanged prompt, guard, source-preparation, tool and event
// adapters through real Pi in a separate HTTP worker. Core has no Pi path or
// scratch directory; all worker storage is private to the remote runner.
func TestRemoteManagerRealPiToolsGuardQuestionsAndResume(t *testing.T) {
	pi, err := exec.LookPath("pi")
	if err != nil {
		t.Skip("pi not installed")
	}
	manager := newLimitedManager(t)
	provider := httptest.NewServer(mockllm.Handler())
	defer provider.Close()
	const token = "remote-worker-authentication-0123456789abcdef"
	worker, err := agentexec.NewService(agentexec.ServiceConfig{LocalAssignments: true, Token: token, Runner: agentexec.Local{PiBin: pi, WorkRoot: t.TempDir()}})
	require.NoError(t, err)
	server := httptest.NewServer(worker)
	defer server.Close()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = worker.Shutdown(ctx)
	})
	var mu sync.Mutex
	var source, widget, title string
	var revision int
	state := map[string]string{}
	var sourceReads atomic.Int32
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		grant := manager.cfg.Credentials.Resolve(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		if grant == nil || grant.Scope().OwnerID != 7 {
			http.Error(w, "forbidden", 403)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		artifact := func() map[string]any {
			return map[string]any{"id": "artifact", "title": title, "body": source, "source_revision": fmt.Sprint(revision), "widget_revision": "w1", "network_allowlist": []string{}}
		}
		var data struct {
			Title, Body, Key, Value string
			ExpectedSourceRevision  string `json:"expected_source_revision"`
			Edits                   []struct{ OldText, NewText string }
		}
		if r.Method == "POST" || r.Method == "PATCH" || r.Method == "PUT" {
			if json.NewDecoder(r.Body).Decode(&data) != nil {
				http.Error(w, "invalid body", 400)
				return
			}
		}
		switch {
		case r.URL.Path == "/api/artifacts" && r.Method == "POST":
			title, source, revision = data.Title, data.Body, 1
			grant.BindArtifact("artifact")
			_ = manager.st.PutArtifact(r.Context(), &store.Artifact{ID: "artifact", OwnerID: 7, Title: title, SourceBlobID: "body", Tier: 1})
			json.NewEncoder(w).Encode(map[string]any{"artifact": artifact(), "render_url": "/a/artifact"})
		case grant.Scope().ArtifactID != "artifact":
			http.Error(w, "wrong scope", 403)
		case strings.HasSuffix(r.URL.Path, "/assets"):
			fmt.Fprint(w, `{"assets":[]}`)
		case strings.HasSuffix(r.URL.Path, "/state"):
			if r.Method == "PUT" {
				state[data.Key] = data.Value
			}
			json.NewEncoder(w).Encode(state)
		case strings.HasSuffix(r.URL.Path, "/widget"):
			if r.Method == "PUT" {
				widget = data.Body
			}
			json.NewEncoder(w).Encode(map[string]any{"body": widget, "widget_revision": "w1", "unapproved": []string{}})
		case r.Method == "PATCH":
			if data.ExpectedSourceRevision != fmt.Sprint(revision) {
				http.Error(w, "conflict", 409)
				return
			}
			if data.Body != "" {
				source = data.Body
			}
			for _, edit := range data.Edits {
				source = strings.Replace(source, edit.OldText, edit.NewText, 1)
			}
			revision++
			json.NewEncoder(w).Encode(map[string]any{"artifact": artifact(), "render_url": "/a/artifact"})
		default:
			if r.URL.Query().Get("body") == "true" {
				sourceReads.Add(1)
			}
			json.NewEncoder(w).Encode(artifact())
		}
	}))
	defer api.Close()
	manager.cfg.PiBin = "/missing/core/pi"
	manager.cfg.WorkRoot = "/missing/core/scratch"
	manager.cfg.APIBaseURL = api.URL
	manager.cfg.MockLLMURL = provider.URL
	manager.cfg.Guardrail = &Guardrail{Provider: "exhibit-mock", Model: "exhibit-mock-1", APIKey: "guard-key"}
	manager.cfg.Runner = agentexec.Remote{Endpoint: agentexec.Endpoint{URL: server.URL, Token: token, AllowUnassigned: true}}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = manager.Shutdown(ctx)
	}()
	var checkpoints atomic.Int32
	manager.cfg.ReserveExecution = func(_ context.Context, id agentexec.Identity, original *agentscope.Grant) (*ExecutionReservation, error) {
		grant, err := manager.cfg.Credentials.Issue(id.OwnerID, id.ArtifactID, id.SessionID)
		return &ExecutionReservation{Grant: grant, Checkpoint: func(ctx context.Context, c ExecutionCheckpoint) error {
			checkpoints.Add(1)
			if grant.Scope().ArtifactID == "" || c.SessionFile == "" {
				return nil
			}
			return manager.st.SaveTranscript(ctx, 7, store.Transcript{ArtifactID: grant.Scope().ArtifactID, SessionID: id.SessionID, Title: c.Title, SessionFile: c.SessionFile})
		}}, err
	}
	create := func(resume string) *Session {
		opts := CreateOpts{OwnerID: 7, Provider: "exhibit-mock", Model: "exhibit-mock-1", APIKey: "model-key", ResumeSessionID: resume}
		if resume != "" {
			opts.ArtifactID = "artifact"
		}
		s, err := manager.Create(context.Background(), opts)
		require.NoError(t, err)
		return s
	}
	send := func(s *Session, text string, want string) {
		s.mu.Lock()
		count := len(s.backlog)
		s.mu.Unlock()
		events, unsubscribe := s.Subscribe()
		defer unsubscribe()
		for range count {
			select {
			case <-events:
			case <-time.After(5 * time.Second):
				t.Fatal("replay did not drain")
			}
		}
		require.NoError(t, s.Prompt(context.Background(), text, nil, nil))
		deadline := time.After(15 * time.Second)
		found := want == ""
		for {
			select {
			case event := <-events:
				line := string(event)
				if strings.Contains(line, want) {
					found = true
				}
				if strings.Contains(line, `"type":"agent_settled"`) || strings.Contains(line, `"type":"exhibit_guard_blocked"`) {
					require.True(t, found, "missing event %s", want)
					return
				}
			case <-deadline:
				t.Fatal("remote Pi never settled")
			}
		}
	}
	s := create("")
	assert.Empty(t, s.workDir)
	assert.Nil(t, s.cmd)
	send(s, "build a counter", "exhibit_artifact_saved")
	assert.Equal(t, "artifact", s.ArtifactID())
	send(s, "make it blue", "exhibit_artifact_saved")
	mu.Lock()
	assert.Contains(t, source, "#3b82f6")
	mu.Unlock()
	send(s, "set state count to 7", "exhibit_state_changed")
	mu.Lock()
	assert.Equal(t, "7", state["count"])
	mu.Unlock()
	send(s, "mock-policy-violation", "exhibit_guard_blocked")
	// Ask-user uses the same waiting RPC dialog over the remote transport.
	s.mu.Lock()
	questionReplay := len(s.backlog)
	s.mu.Unlock()
	events, unsubscribe := s.Subscribe()
	for range questionReplay {
		<-events
	}
	require.NoError(t, s.Prompt(context.Background(), "mock-ask-questions", nil, nil))
	var request QuestionRequest
	require.Eventually(t, func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		for _, q := range s.questions {
			if q.Pending {
				request = q
				return true
			}
		}
		return false
	}, 15*time.Second, 10*time.Millisecond)
	require.NoError(t, s.AnswerQuestions(request.ID, QuestionResponse{Message: "A weekly view"}))
	deadline := time.After(10 * time.Second)
	for {
		select {
		case event := <-events:
			if strings.Contains(string(event), `"type":"agent_settled"`) {
				goto answered
			}
		case <-deadline:
			t.Fatal("question answer never settled")
		}
	}
answered:
	unsubscribe()
	conversation := s.ID
	manager.Close(7, s.ID)
	waitForConversationRelease(t, manager, s.ID)
	resumed := create(conversation)
	require.Eventually(t, func() bool { return sourceReads.Load() > 0 }, 10*time.Second, time.Millisecond)
	send(resumed, "list state", "")
	widgetSession, err := manager.Create(context.Background(), CreateOpts{OwnerID: 7, Provider: "exhibit-mock", Model: "exhibit-mock-1", APIKey: "model-key", ArtifactID: "artifact", WidgetOnly: true})
	require.NoError(t, err)
	send(widgetSession, "build the widget", "exhibit_widget_saved")
	mu.Lock()
	assert.NotEmpty(t, widget)
	mu.Unlock()
	assert.Greater(t, checkpoints.Load(), int32(4))
	require.NoError(t, manager.Shutdown(context.Background()))
}
