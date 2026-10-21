package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/momja/Exhibit/internal/agentcontrol"
	"github.com/momja/Exhibit/internal/agentscope"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type workerAuthorityFixture struct {
	ro         *Router
	c          *agentcontrol.Control
	handler    http.Handler
	assignment agentcontrol.Assignment
	token      string
	grant      *agentscope.Grant
	clock      atomic.Int64
}

func newWorkerAuthorityFixture(t *testing.T) *workerAuthorityFixture {
	t.Helper()
	ro := newTestRouter(t)
	id := createArtifact(t, ro, map[string]any{"title": "Worker artifact", "body": "<p>one</p>"})
	f := &workerAuthorityFixture{ro: ro}
	f.clock.Store(time.Now().UnixMilli())
	f.c = agentcontrol.New(ro.cfg.Store.(agentcontrol.Repository), agentscope.NewRegistry(), agentcontrol.Config{LeaseTTL: time.Minute, SupervisorTTL: time.Hour, Now: func() time.Time { return time.UnixMilli(f.clock.Load()) }})
	var err error
	f.assignment, err = f.c.Reserve(context.Background(), agentcontrol.Reservation{OwnerID: 1, ConversationID: "worker-conversation", ExecutionID: "worker-execution", WorkerID: "worker-1", ArtifactID: id})
	require.NoError(t, err)
	f.token, err = f.c.IssueSupervisor(context.Background(), f.assignment.Identity, "platform")
	require.NoError(t, err)
	f.grant, err = f.c.IssueTool(context.Background(), f.assignment.Identity)
	require.NoError(t, err)
	f.handler = NewAgentWorkerHandler(f.c)
	return f
}
func (f *workerAuthorityFixture) send(method, path, token string, body io.Reader, revision string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, body)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set(AgentCheckpointRevisionHeader, revision)
	req.Header.Set(AgentCheckpointKindHeader, agentcontrol.CheckpointSettled)
	req.Header.Set(AgentConversationTitleHeader, "Worker title")
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, req)
	return w
}
func (f *workerAuthorityFixture) save(t *testing.T, revision string, file string) *httptest.ResponseRecorder {
	t.Helper()
	w := f.send("PUT", AgentWorkerAuthorityPath+"/snapshot", f.token, strings.NewReader(file), revision)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	return w
}

func TestWorkerAuthoritySeparatesSupervisorFromBrowserServiceAndToolCredentials(t *testing.T) {
	f := newWorkerAuthorityFixture(t)
	for _, token := range []string{"", "secret", f.grant.Token(), "exsuper_" + strings.Repeat("0", 64)} {
		for _, method := range []string{"GET", "PUT", "POST"} {
			path := AgentWorkerAuthorityPath + "/snapshot"
			if method == "POST" {
				path = AgentWorkerAuthorityPath + "/usage"
			}
			w := f.send(method, path, token, strings.NewReader(piFile), "1")
			assert.Equal(t, http.StatusUnauthorized, w.Code, w.Body.String())
			if token != "" {
				assert.NotContains(t, w.Body.String(), token)
			}
		}
	}
	req := httptest.NewRequest("GET", AgentWorkerAuthorityPath+"/snapshot", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "browser-session"})
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	f.save(t, "1", piFile)
	w = f.send("GET", AgentWorkerAuthorityPath+"/snapshot", f.token, nil, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, piFile, w.Body.String())
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	assert.Equal(t, "application/x-ndjson", w.Header().Get("Content-Type"))
	for _, path := range []string{"snapshot?owner_id=2&artifact_id=foreign", "snapshot?conversation_id=another"} {
		w = f.send("GET", AgentWorkerAuthorityPath+"/"+path, f.token, nil, "")
		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, piFile, w.Body.String(), "query identities cannot widen assigned authority")
	}
	// The exact same durable row is read by the existing browser History code,
	// which still publishes only its safe projection.
	history := doJSON(t, f.ro, "GET", "/api/artifacts/"+f.assignment.ArtifactID+"/transcripts/"+f.assignment.ConversationID, nil)
	require.Equal(t, http.StatusOK, history.Code, history.Body.String())
	for _, secret := range []string{"session_file", "SECRET-CWD", "SECRET-SYSTEM-PROMPT", "SECRET-ARTIFACT-SOURCE", "SECRET-PROVIDER", "SECRET-MODEL", f.token} {
		assert.NotContains(t, history.Body.String(), secret)
	}
}

func TestWorkerAuthorityCheckpointRetriesOrderAndReassignment(t *testing.T) {
	f := newWorkerAuthorityFixture(t)
	w := f.save(t, "2", piFile)
	assert.Equal(t, "2", w.Header().Get(AgentCheckpointRevisionHeader))
	assert.Equal(t, "1", w.Header().Get(AgentArtifactVersionHeader))
	assert.NotContains(t, w.Body.String(), "SECRET")
	f.save(t, "2", piFile)
	w = f.send("PUT", AgentWorkerAuthorityPath+"/snapshot", f.token, strings.NewReader(piFile), "1")
	assert.Equal(t, http.StatusConflict, w.Code)
	w = f.send("PUT", AgentWorkerAuthorityPath+"/snapshot", f.token, strings.NewReader(piFile+"{\"type\":\"other\"}\n"), "2")
	assert.Equal(t, http.StatusConflict, w.Code)
	for _, file := range []string{"{\"type\":\"partial\"}", "[1,2]\n", "bad\n"} {
		w = f.send("PUT", AgentWorkerAuthorityPath+"/snapshot", f.token, strings.NewReader(file), "3")
		assert.Equal(t, http.StatusBadRequest, w.Code)
	}
	require.NoError(t, f.c.BeginClose(context.Background(), f.assignment.Identity))
	f.save(t, "3", piFile)
	require.NoError(t, f.c.Release(context.Background(), f.assignment.Identity, agentcontrol.Closed))
	_, err := f.c.Reserve(context.Background(), agentcontrol.Reservation{OwnerID: 1, ArtifactID: f.assignment.ArtifactID, ConversationID: f.assignment.ConversationID, ExecutionID: "replacement", WorkerID: "worker-2", Resume: true})
	require.NoError(t, err)
	for _, method := range []string{"GET", "PUT"} {
		w = f.send(method, AgentWorkerAuthorityPath+"/snapshot", f.token, strings.NewReader(piFile), "4")
		assert.Equal(t, http.StatusConflict, w.Code)
	}
}

func TestWorkerAuthoritySnapshotBodyBoundAppliesBeforeReadAndWithoutContentLength(t *testing.T) {
	f := newWorkerAuthorityFixture(t)
	for _, declared := range []int64{agentcontrol.MaxSnapshotBytes + 1, -1} {
		body := &countingBody{remaining: agentcontrol.MaxSnapshotBytes + 200}
		req := httptest.NewRequest("PUT", AgentWorkerAuthorityPath+"/snapshot", body)
		req.ContentLength = declared
		req.Header.Set("Authorization", "Bearer "+f.token)
		req.Header.Set(AgentCheckpointRevisionHeader, "1")
		req.Header.Set(AgentCheckpointKindHeader, agentcontrol.CheckpointSettled)
		w := httptest.NewRecorder()
		f.handler.ServeHTTP(w, req)
		require.Equal(t, http.StatusRequestEntityTooLarge, w.Code, w.Body.String())
		if declared > 0 {
			assert.Zero(t, body.read)
		} else {
			assert.Equal(t, int64(agentcontrol.MaxSnapshotBytes+1), body.read)
		}
	}
}

func TestWorkerAuthorityAcceptsRawSnapshotBeyondGeneric32MiBLimit(t *testing.T) {
	f := newWorkerAuthorityFixture(t)
	// Raw JSONL avoids a second JSON string escaping layer and has its own 64 MiB
	// limit. This request would be rejected by the browser API's generic limiter.
	file := "{\"type\":\"message\",\"payload\":\"" + strings.Repeat("x", 33<<20) + "\"}\n"
	f.save(t, "1", file)
	got, err := f.c.ReadSnapshot(context.Background(), f.assignment.Identity)
	require.NoError(t, err)
	assert.Equal(t, len(file), len(got.SessionFile))
}

func TestWorkerAuthorityLateUsageDedupFundingAndStrictInput(t *testing.T) {
	f := newWorkerAuthorityFixture(t)
	require.NoError(t, f.c.Release(context.Background(), f.assignment.Identity, agentcontrol.Failed))
	report := `{"event_id":"message-17","source":"model","provider":"p","model":"m","input_tokens":20,"output_tokens":3,"cost_micros":125}`
	for i := 0; i < 2; i++ {
		w := f.send("POST", AgentWorkerAuthorityPath+"/usage", f.token, strings.NewReader(report), "")
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var got struct {
			Recorded bool `json:"recorded"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
		assert.Equal(t, i == 0, got.Recorded)
	}
	sum, err := f.ro.cfg.Store.SessionAgentSpend(context.Background(), 1, f.assignment.ConversationID)
	require.NoError(t, err)
	assert.Equal(t, int64(125), sum.CostMicros)
	for _, body := range []string{
		`{"event_id":"x","owner_id":2,"source":"model"}`,
		`{"event_id":"x","paid_by":"user","source":"model"}`,
		`{"event_id":"x","generation":77,"source":"model"}`,
		`{"event_id":"x","source":"model","input_tokens":-1}`,
		report + ` {}`,
	} {
		w := f.send("POST", AgentWorkerAuthorityPath+"/usage", f.token, strings.NewReader(body), "")
		assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	}
	changed := strings.Replace(report, `"input_tokens":20`, `"input_tokens":21`, 1)
	w := f.send("POST", AgentWorkerAuthorityPath+"/usage", f.token, strings.NewReader(changed), "")
	assert.Equal(t, http.StatusConflict, w.Code)
	body := &countingBody{remaining: agentcontrol.MaxUsageReportBytes + 100}
	req := httptest.NewRequest("POST", AgentWorkerAuthorityPath+"/usage", body)
	req.ContentLength = -1
	req.Header.Set("Authorization", "Bearer "+f.token)
	w = httptest.NewRecorder()
	f.handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
	assert.LessOrEqual(t, body.read, int64(agentcontrol.MaxUsageReportBytes+1))
	f.clock.Add(int64((2 * time.Hour) / time.Millisecond))
	w = f.send("POST", AgentWorkerAuthorityPath+"/usage", f.token, strings.NewReader(report), "")
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestWorkerAuthorityLeaseExpiryAndCredentialRotation(t *testing.T) {
	f := newWorkerAuthorityFixture(t)
	f.save(t, "1", piFile)
	rotated, err := f.c.IssueSupervisor(context.Background(), f.assignment.Identity, "platform")
	require.NoError(t, err)
	w := f.send("GET", AgentWorkerAuthorityPath+"/snapshot", f.token, nil, "")
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	f.token = rotated
	f.clock.Add(61 * 1000)
	w = f.send("GET", AgentWorkerAuthorityPath+"/snapshot", f.token, nil, "")
	assert.Equal(t, http.StatusConflict, w.Code)
	report := `{"event_id":"after-lease","source":"tool","input_tokens":2}`
	w = f.send("POST", AgentWorkerAuthorityPath+"/usage", f.token, strings.NewReader(report), "")
	assert.Equal(t, http.StatusOK, w.Code)
	w = f.send("GET", "/internal/agent-worker/v2/snapshot", f.token, nil, "")
	assert.Equal(t, http.StatusNotFound, w.Code)
	w = f.send("DELETE", AgentWorkerAuthorityPath+"/snapshot", f.token, nil, "")
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestWorkerAuthorityLegacyMessageDumpRemainsReadOnly(t *testing.T) {
	ro, _, path := newTestRouterAt(t)
	id := createArtifact(t, ro, map[string]any{"title": "old", "body": "<p>old</p>"})
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	defer db.Close()
	_, err = db.Exec(`INSERT INTO agent_transcripts(artifact_id,session_id,messages) VALUES(?,?,'[{"role":"user","content":"before session files"}]')`, id, "legacy")
	require.NoError(t, err)
	c := agentcontrol.New(ro.cfg.Store.(agentcontrol.Repository), agentscope.NewRegistry(), agentcontrol.Config{})
	_, err = c.Reserve(context.Background(), agentcontrol.Reservation{OwnerID: 1, ArtifactID: id, ConversationID: "legacy", ExecutionID: "legacy-attempt", WorkerID: "worker", Resume: true})
	require.ErrorIs(t, err, agentcontrol.ErrNotResumable)
	got := doJSON(t, ro, "GET", "/api/artifacts/"+id+"/transcripts/legacy", nil)
	require.Equal(t, http.StatusOK, got.Code, got.Body.String())
	assert.Contains(t, got.Body.String(), "before session files")
	assert.NotContains(t, got.Body.String(), "session_file")
}

func TestWorkerAuthorityPolicyStorageOutageFailsClosed(t *testing.T) {
	f := newWorkerAuthorityFixture(t)
	require.NoError(t, f.ro.cfg.Store.Close())
	for _, method := range []string{"GET", "PUT", "POST"} {
		path := AgentWorkerAuthorityPath + "/snapshot"
		if method == "POST" {
			path = AgentWorkerAuthorityPath + "/usage"
		}
		w := f.send(method, path, f.token, strings.NewReader(piFile), "1")
		assert.Equal(t, http.StatusServiceUnavailable, w.Code)
		assert.Equal(t, "worker authority unavailable\n", w.Body.String())
		assert.NotContains(t, w.Body.String(), f.token)
	}
}

func TestInsertedLateUsageNotifiesCentralPolicyOnce(t *testing.T) {
	f := newWorkerAuthorityFixture(t)
	require.NoError(t, f.c.Release(context.Background(), f.assignment.Identity, agentcontrol.Interrupted))
	var notifications atomic.Int32
	f.handler = NewAgentWorkerHandler(f.c, func(ctx context.Context) {
		total, err := f.ro.cfg.Store.InstanceAgentSpend(ctx, time.Time{})
		require.NoError(t, err)
		require.Equal(t, int64(125), total.CostMicros, "notify after the receipt commits")
		notifications.Add(1)
	})
	report := `{"event_id":"late-message","source":"model","cost_micros":125}`
	for range 2 {
		response := f.send("POST", AgentWorkerAuthorityPath+"/usage", f.token, strings.NewReader(report), "")
		require.Equal(t, 200, response.Code, response.Body.String())
	}
	require.Equal(t, int32(1), notifications.Load(), "idempotent retry must not repeat enforcement")
	invalid := f.send("POST", AgentWorkerAuthorityPath+"/usage", f.token, strings.NewReader(`{"event_id":"invalid","source":"model","cost_micros":-1}`), "")
	require.Equal(t, 400, invalid.Code)
	require.Equal(t, int32(1), notifications.Load())
}
