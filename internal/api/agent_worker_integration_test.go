package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/momja/Exhibit/internal/agentcontrol"
	"github.com/momja/Exhibit/internal/agentscope"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/require"
)

func TestWorkerRoutesBypassLoginAndGeneralBodyLimit(t *testing.T) {
	f := newWorkerAuthorityFixture(t)
	cfg := f.ro.cfg
	cfg.AgentControl = f.c
	cfg.AuthToken = "operator-token"
	cfg.LocalUsers = true
	cfg.MaxRequestBodyBytes = 32
	server := httptest.NewServer(NewRouter(cfg))
	defer server.Close()
	request, err := http.NewRequest("PUT", server.URL+AgentWorkerAuthorityPath+"/snapshot", strings.NewReader(piFile))
	require.NoError(t, err)
	request.Header.Set("Authorization", "Bearer "+f.token)
	request.Header.Set(AgentCheckpointRevisionHeader, "1")
	request.Header.Set(AgentCheckpointKindHeader, "settled")
	response, err := server.Client().Do(request)
	require.NoError(t, err)
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	require.Equal(t, 200, response.StatusCode, string(body))
	request, err = http.NewRequest("GET", server.URL+AgentWorkerAuthorityPath+"/snapshot", nil)
	require.NoError(t, err)
	response, err = server.Client().Do(request)
	require.NoError(t, err)
	response.Body.Close()
	require.Equal(t, 401, response.StatusCode)
	render := httptest.NewRecorder()
	NewRouter(cfg).RenderHandler().ServeHTTP(render, httptest.NewRequest("GET", AgentWorkerAuthorityPath+"/snapshot", nil))
	require.Equal(t, 404, render.Code)
}

func TestUnknownWorkerCredentialsCannotBecomeAnonymous(t *testing.T) {
	ro := newTestRouter(t)
	ro.cfg.AuthToken = ""
	for _, token := range []string{"exagent_retired", "exsuper_retired"} {
		response := httptest.NewRecorder()
		request := httptest.NewRequest("GET", "/api/artifacts", nil)
		request.Header.Set("Authorization", "Bearer "+token)
		ro.ServeHTTP(response, request)
		require.Equal(t, 401, response.Code)
	}
}

func TestFencedConcurrentCreateBindsExactlyOnceAndRejectsSupersededWrites(t *testing.T) {
	ro := newTestRouter(t)
	registry := agentscope.NewRegistry()
	control := agentcontrol.New(ro.cfg.Store.(agentcontrol.Repository), registry, agentcontrol.Config{})
	ro.cfg.AgentCredentials = registry
	ro.cfg.AgentControl = control
	ctx := context.Background()
	assignment, err := control.Reserve(ctx, agentcontrol.Reservation{OwnerID: 1, ConversationID: "fenced-create", ExecutionID: "attempt-one", WorkerID: "worker-one"})
	require.NoError(t, err)
	grant, err := control.IssueTool(ctx, assignment.Identity)
	require.NoError(t, err)
	server := httptest.NewServer(ro)
	defer server.Close()
	codes := make(chan int, 2)
	var group sync.WaitGroup
	for range 2 {
		group.Add(1)
		go func() {
			defer group.Done()
			request, _ := http.NewRequest("POST", server.URL+"/api/artifacts", strings.NewReader(`{"title":"created once","body":"<p>safe</p>"}`))
			request.Header.Set("Authorization", "Bearer "+grant.Token())
			response, err := server.Client().Do(request)
			if err != nil {
				codes <- 0
				return
			}
			defer response.Body.Close()
			codes <- response.StatusCode
		}()
	}
	group.Wait()
	close(codes)
	count := map[int]int{}
	for code := range codes {
		count[code]++
	}
	require.Equal(t, 1, count[201], count)
	require.Equal(t, 1, count[403]+count[401], count)
	artifact := grant.Scope().ArtifactID
	require.NotEmpty(t, artifact)
	_, err = control.WriteCheckpoint(ctx, assignment.Identity, agentcontrol.Checkpoint{Revision: 1, Kind: "settled", SessionFile: piFile})
	require.NoError(t, err)
	require.NoError(t, control.Release(ctx, assignment.Identity, agentcontrol.Interrupted))
	replacement, err := control.Reserve(ctx, agentcontrol.Reservation{OwnerID: 1, ArtifactID: artifact, ConversationID: "fenced-create", ExecutionID: "attempt-two", WorkerID: "worker-two", Resume: true})
	require.NoError(t, err)
	require.Greater(t, replacement.Generation, assignment.Generation)
	response := httptest.NewRecorder()
	request := httptest.NewRequest("PATCH", "/api/artifacts/"+artifact, strings.NewReader(`{"title":"stale write"}`))
	request.Header.Set("Authorization", "Bearer "+grant.Token())
	ro.ServeHTTP(response, request)
	require.Equal(t, 401, response.Code)
	snapshot, err := control.ReadSnapshot(ctx, replacement.Identity)
	require.NoError(t, err)
	require.Equal(t, 1, snapshot.VersionSeq)
}

func TestToolHandlerGuardBlocksTakeoverUntilHandlerFinishes(t *testing.T) {
	ro := newTestRouter(t)
	registry := agentscope.NewRegistry()
	control := agentcontrol.New(ro.cfg.Store.(agentcontrol.Repository), registry, agentcontrol.Config{})
	ro.cfg.AgentCredentials = registry
	assignment, err := control.Reserve(context.Background(), agentcontrol.Reservation{OwnerID: 1, ConversationID: "guard", ExecutionID: "attempt", WorkerID: "worker"})
	require.NoError(t, err)
	grant, err := control.IssueTool(context.Background(), assignment.Identity)
	require.NoError(t, err)
	entered, finish, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	handler := ro.authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(entered); <-finish }))
	go func() {
		defer close(done)
		request := httptest.NewRequest("POST", "/api/artifacts", nil)
		request.Header.Set("Authorization", "Bearer "+grant.Token())
		handler.ServeHTTP(httptest.NewRecorder(), request)
	}()
	<-entered
	released := make(chan error, 1)
	go func() {
		released <- control.Release(context.Background(), assignment.Identity, agentcontrol.Interrupted)
	}()
	select {
	case err := <-released:
		t.Fatalf("authority released during handler: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	close(finish)
	<-done
	require.NoError(t, <-released)
}

func TestWorkerCheckpointUsesCommittedSourceAndWidgetVersions(t *testing.T) {
	ro := newTestRouter(t)
	artifact := createArtifact(t, ro, map[string]any{"title": "versioned", "body": "<p>initial</p>"})
	registry := agentscope.NewRegistry()
	control := agentcontrol.New(ro.cfg.Store.(agentcontrol.Repository), registry, agentcontrol.Config{})
	ro.cfg.AgentControl, ro.cfg.AgentCredentials = control, registry
	assignment, err := control.Reserve(context.Background(), agentcontrol.Reservation{OwnerID: 1, ConversationID: "versioned-conversation", ExecutionID: "versioned-attempt", WorkerID: "versioned-worker", ArtifactID: artifact})
	require.NoError(t, err)
	grant, err := control.IssueTool(context.Background(), assignment.Identity)
	require.NoError(t, err)
	send := func(method, path, token, body, revision string) *httptest.ResponseRecorder {
		if body != "" {
			var fields map[string]any
			require.NoError(t, json.Unmarshal([]byte(body), &fields))
			name := "expected_source_revision"
			if strings.HasSuffix(path, "/widget") {
				name = "expected_widget_revision"
			}
			fields[name] = revision
			encoded, err := json.Marshal(fields)
			require.NoError(t, err)
			body = string(encoded)
		}
		request := httptest.NewRequest(method, path, strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer "+token)
		if revision != "" {
			request.Header.Set("If-Match", revision)
		}
		response := httptest.NewRecorder()
		ro.ServeHTTP(response, request)
		return response
	}
	row, err := ro.cfg.Store.GetArtifact(context.Background(), 1, artifact)
	require.NoError(t, err)
	response := send("PATCH", "/api/artifacts/"+artifact, grant.Token(), `{"body":"<p>agent source</p>"}`, store.ContentRevision(row.SourceBlobID))
	require.Equal(t, 200, response.Code, response.Body.String())
	response = send("PUT", "/api/artifacts/"+artifact+"/widget", grant.Token(), `{"body":"<p>agent widget</p>"}`, store.ContentRevision(""))
	require.Equal(t, 200, response.Code, response.Body.String())
	row, err = ro.cfg.Store.GetArtifact(context.Background(), 1, artifact)
	require.NoError(t, err)
	// A browser save moves head after the worker's write, before its snapshot.
	response = send("PATCH", "/api/artifacts/"+artifact, ro.cfg.AuthToken, `{"body":"<p>browser source</p>"}`, store.ContentRevision(row.SourceBlobID))
	require.Equal(t, 200, response.Code, response.Body.String())
	snapshot, err := control.WriteCheckpoint(context.Background(), assignment.Identity, agentcontrol.Checkpoint{Revision: 1, Kind: "settled", SessionFile: piFile})
	require.NoError(t, err)
	require.Equal(t, 3, snapshot.VersionSeq)
	response = send("DELETE", "/api/artifacts/"+artifact+"/widget", grant.Token(), "", store.ContentRevision(row.WidgetBlobID))
	require.Equal(t, 204, response.Code, response.Body.String())
	snapshot, err = control.WriteCheckpoint(context.Background(), assignment.Identity, agentcontrol.Checkpoint{Revision: 2, Kind: "settled", SessionFile: piFile})
	require.NoError(t, err)
	require.Equal(t, 5, snapshot.VersionSeq)
}
