package mcpserver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/momja/Exhibit/internal/agentscope"
	"github.com/momja/Exhibit/internal/api"
	"github.com/momja/Exhibit/internal/blob"
	"github.com/momja/Exhibit/internal/mcpserver"
	"github.com/momja/Exhibit/internal/secrets"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const remoteOperatorToken = "remote-test-operator"
const initializeMessage = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"remote-test","version":"test"}}}`

type remoteFixture struct {
	core               *httptest.Server
	database           *store.SQLiteStore
	registry           *agentscope.Registry
	credentialRequests atomic.Int64
	cookieRequests     atomic.Int64
}

func newRemoteFixture(t *testing.T) *remoteFixture {
	t.Helper()
	dir := t.TempDir()
	database, err := store.OpenSQLite(filepath.Join(dir, "exhibit.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	blobs, err := blob.NewFSStore(filepath.Join(dir, "blobs"))
	require.NoError(t, err)
	box, err := secrets.Load("remote-test-secret", "")
	require.NoError(t, err)
	f := &remoteFixture{database: database, registry: agentscope.NewRegistry()}
	router := api.NewRouter(api.Config{Store: database, Blob: blobs, Secrets: box,
		MCPURL: "http://mcp.test/mcp", AuthToken: remoteOperatorToken, AppOrigin: "http://app.test", RenderOrigin: "http://render.test", AgentCredentials: f.registry})
	f.core = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/agent/credential" {
			f.credentialRequests.Add(1)
		}
		if r.Header.Get("Cookie") != "" {
			f.cookieRequests.Add(1)
		}
		router.ServeHTTP(w, r)
	}))
	t.Cleanup(f.core.Close)
	return f
}

func (f *remoteFixture) request(t *testing.T, method, path string, body any, status int, out any) {
	t.Helper()
	payload, err := json.Marshal(body)
	require.NoError(t, err)
	req, err := http.NewRequest(method, f.core.URL+path, bytes.NewReader(payload))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+remoteOperatorToken)
	req.Header.Set("Content-Type", "application/json")
	response, err := f.core.Client().Do(req)
	require.NoError(t, err)
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Equal(t, status, response.StatusCode, "%s", data)
	if out != nil {
		require.NoError(t, json.Unmarshal(data, out))
	}
}

type remoteGrant struct {
	Token string           `json:"token"`
	Grant store.AgentGrant `json:"grant"`
}

func (f *remoteFixture) grant(t *testing.T, label string) remoteGrant {
	t.Helper()
	var grant remoteGrant
	f.request(t, "POST", "/api/agent-grants", map[string]string{"label": label}, http.StatusCreated, &grant)
	return grant
}

func (f *remoteFixture) service(t *testing.T, limit int64) *httptest.Server {
	t.Helper()
	handler, err := mcpserver.NewHTTPHandler(mcpserver.HTTPConfig{BaseURL: f.core.URL, PublicURL: "http://mcp.test/mcp", Version: "test", MaxRequestBodyBytes: limit})
	require.NoError(t, err)
	service := httptest.NewServer(handler)
	t.Cleanup(service.Close)
	return service
}

type remoteHeaders struct{ header http.Header }

func (h remoteHeaders) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	for name, values := range h.header {
		r.Header[name] = values
	}
	return http.DefaultTransport.RoundTrip(r)
}

func connectRemote(t *testing.T, endpoint, token string) *mcp.ClientSession {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	client := mcp.NewClient(&mcp.Implementation{Name: "remote-test", Version: "test"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: endpoint,
		HTTPClient: &http.Client{Transport: remoteHeaders{http.Header{
			"Authorization": {"Bearer " + token}, "Cookie": {"exhibit_session=must-not-be-forwarded"},
		}}}}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func remoteCall(t *testing.T, session *mcp.ClientSession, name string, args map[string]any) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	require.NoError(t, err)
	require.False(t, result.IsError, "%+v", result.Content)
	var text strings.Builder
	for _, content := range result.Content {
		if c, ok := content.(*mcp.TextContent); ok {
			text.WriteString(c.Text)
		}
	}
	return text.String()
}

func remoteRequest(t *testing.T, endpoint, method, message string, headers http.Header) (int, http.Header, string) {
	t.Helper()
	req, err := http.NewRequest(method, endpoint, strings.NewReader(message))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	for name, values := range headers {
		req.Header[name] = values
	}
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	require.NoError(t, err)
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	return response.StatusCode, response.Header, string(data)
}

func TestRemoteMCPToolsUseTheCoreAPIAndGrantProvenance(t *testing.T) {
	f := newRemoteFixture(t)
	grant := f.grant(t, "Claude Code remote")
	var created struct {
		Artifact store.Artifact `json:"artifact"`
	}
	f.request(t, "POST", "/api/artifacts", map[string]string{"title": "Water tracker", "body": "<html><body><p>Water today: 2</p></body></html>"}, http.StatusCreated, &created)
	service := f.service(t, 0)
	session := connectRemote(t, service.URL+"/mcp", grant.Token)
	require.Contains(t, remoteCall(t, session, "search_artifacts", map[string]any{"query": "water tracker"}), created.Artifact.ID)
	require.Contains(t, remoteCall(t, session, "get_artifact", map[string]any{"artifact_id": created.Artifact.ID}), "Water today: 2")
	remoteCall(t, session, "edit_artifact", map[string]any{"artifact_id": created.Artifact.ID, "edits": []map[string]string{{"oldText": "Water today: 2", "newText": "Water today: 3"}}})
	require.Contains(t, remoteCall(t, session, "get_artifact", map[string]any{"artifact_id": created.Artifact.ID}), "Water today: 3")
	versions, err := f.database.ListVersions(context.Background(), 1, created.Artifact.ID)
	require.NoError(t, err)
	require.NotEmpty(t, versions)
	assert.Equal(t, store.VersionAgent, versions[0].Origin)
	assert.Equal(t, grant.Grant.Label, versions[0].Message)
	assert.Equal(t, "grant:"+grant.Grant.ID, versions[0].SessionID)
	// Credential checks, protocol calls and tools must all ignore browser cookies.
	assert.Zero(t, f.cookieRequests.Load())
	before := f.credentialRequests.Load()
	_, err = session.ListTools(context.Background(), nil)
	require.NoError(t, err)
	assert.Greater(t, f.credentialRequests.Load(), before, "even tools/list must verify the grant afresh")
}

func TestRemoteMCPOnlyAcceptsLibraryBearersAndOwnOrigin(t *testing.T) {
	f := newRemoteFixture(t)
	grant := f.grant(t, "remote")
	pi, err := f.registry.Issue(1, "artifact", "pi-session")
	require.NoError(t, err)
	owner, err := f.database.UpsertUser(context.Background(), "remote-cookie-owner", "owner@example.test")
	require.NoError(t, err)
	require.NoError(t, f.database.CreateSession(context.Background(), &store.Session{ID: "valid-owner-session", UserID: owner.ID, ExpiresAt: time.Now().Add(time.Hour)}))
	service := f.service(t, 0)
	for _, tc := range []struct {
		name    string
		headers http.Header
		want    int
	}{
		{"none", nil, 401},
		{"cookie only", http.Header{"Cookie": {"exhibit_session=valid-owner-session"}}, 401},
		{"operator", http.Header{"Authorization": {"Bearer " + remoteOperatorToken}}, 401},
		{"Pi", http.Header{"Authorization": {"Bearer " + pi.Token()}}, 401},
		{"garbage", http.Header{"Authorization": {"Bearer exgrant_forged"}}, 401},
		{"foreign origin", http.Header{"Authorization": {"Bearer " + grant.Token}, "Origin": {"https://foreign.test"}}, 403},
		{"own origin", http.Header{"Authorization": {"Bearer " + grant.Token}, "Origin": {"http://mcp.test"}}, 200},
		{"no origin", http.Header{"Authorization": {"Bearer " + grant.Token}}, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, _, body := remoteRequest(t, service.URL+"/mcp", "POST", initializeMessage, tc.headers)
			assert.Equal(t, tc.want, status, body)
		})
	}
}

func TestRemoteMCPSessionIsBoundToGrantAndRevocationIsImmediate(t *testing.T) {
	f := newRemoteFixture(t)
	grant, other := f.grant(t, "first"), f.grant(t, "second")
	service := f.service(t, 0)
	session := connectRemote(t, service.URL+"/mcp", grant.Token)
	message := `{"jsonrpc":"2.0","id":9,"method":"tools/list"}`
	headers := http.Header{"Authorization": {"Bearer " + other.Token}, "Mcp-Session-Id": {session.ID()}}
	status, _, body := remoteRequest(t, service.URL+"/mcp", "POST", message, headers)
	require.Equal(t, http.StatusForbidden, status, body)
	f.request(t, "DELETE", "/api/agent-grants/"+grant.Grant.ID, nil, http.StatusNoContent, nil)
	headers.Set("Authorization", "Bearer "+grant.Token)
	for _, method := range []string{"POST", "GET", "DELETE"} {
		status, _, body = remoteRequest(t, service.URL+"/mcp", method, message, headers)
		require.Equal(t, http.StatusUnauthorized, status, body)
	}
	_, err := session.ListTools(context.Background(), nil)
	require.Error(t, err)
}

func TestRemoteMCPBoundsKnownAndUnknownRequestBodies(t *testing.T) {
	f := newRemoteFixture(t)
	grant := f.grant(t, "limited")
	service := f.service(t, 256)
	for _, unknown := range []bool{false, true} {
		req, err := http.NewRequest("POST", service.URL+"/mcp", strings.NewReader(initializeMessage+strings.Repeat(" ", 300)))
		require.NoError(t, err)
		if unknown {
			req.ContentLength = -1
		}
		req.Header.Set("Authorization", "Bearer "+grant.Token)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		response, err := service.Client().Do(req)
		require.NoError(t, err)
		response.Body.Close()
		assert.Equal(t, http.StatusRequestEntityTooLarge, response.StatusCode, "unknown length: %v", unknown)
	}
}

func TestRemoteMCPDoesNotAdmitWhenCoreCannotVerify(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"unreachable", 0, ""}, {"server error", 500, `{"scope":"library","grant_id":"fake"}`},
		{"unavailable", 503, "unavailable"}, {"malformed", 200, `{"scope":"library"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			t.Cleanup(core.Close)
			if tc.status == 0 {
				core.Close()
			}
			handler, err := mcpserver.NewHTTPHandler(mcpserver.HTTPConfig{BaseURL: core.URL, PublicURL: "http://mcp.test/mcp", Version: "test"})
			require.NoError(t, err)
			service := httptest.NewServer(handler)
			defer service.Close()
			status, _, body := remoteRequest(t, service.URL+"/mcp", "POST", initializeMessage, http.Header{"Authorization": {"Bearer exgrant_test"}})
			assert.Equal(t, http.StatusServiceUnavailable, status, body)
		})
	}
}

func TestRemoteMCPEndpointPathAndHealth(t *testing.T) {
	f := newRemoteFixture(t)
	grant := f.grant(t, "custom path")
	handler, err := mcpserver.NewHTTPHandler(mcpserver.HTTPConfig{BaseURL: f.core.URL, PublicURL: "http://mcp.test/agent/connect", Version: "test"})
	require.NoError(t, err)
	service := httptest.NewServer(handler)
	defer service.Close()
	status, _, body := remoteRequest(t, service.URL+"/agent/connect", "POST", initializeMessage, http.Header{"Authorization": {"Bearer " + grant.Token}})
	require.Equal(t, 200, status, body)
	status, _, _ = remoteRequest(t, service.URL+"/mcp", "POST", initializeMessage, nil)
	assert.Equal(t, 404, status)
	status, _, _ = remoteRequest(t, service.URL+"/healthz", "GET", "", nil)
	assert.Equal(t, 200, status)
}

func TestRemoteMCPPublicOriginUsesBrowserCanonicalForm(t *testing.T) {
	f := newRemoteFixture(t)
	grant := f.grant(t, "canonical origin")
	handler, err := mcpserver.NewHTTPHandler(mcpserver.HTTPConfig{BaseURL: f.core.URL, PublicURL: "https://MCP.test:443/mcp", Version: "test"})
	require.NoError(t, err)
	service := httptest.NewServer(handler)
	defer service.Close()
	for _, tc := range []struct {
		origin string
		status int
	}{
		{"https://mcp.test", 200},
		{"http://mcp.test", 403},
		{"https://mcp.test:444", 403},
		{"https://mcp.test.", 403},
		{"null", 403},
	} {
		status, _, body := remoteRequest(t, service.URL+"/mcp", "POST", initializeMessage, http.Header{"Authorization": {"Bearer " + grant.Token}, "Origin": {tc.origin}})
		require.Equal(t, tc.status, status, body)
	}
}
