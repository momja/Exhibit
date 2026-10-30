package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/momja/Exhibit/internal/connectors"
	"github.com/momja/Exhibit/internal/egress"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConnectionReconnectEndpointKeepsGrantAndRejectsPolicyChanges(t *testing.T) {
	ro, registry := newScopedTestRouter(t)
	ro.cfg.Connections = connectors.New(connectors.Config{CustomEnabled: true, Store: ro.cfg.Store.(*store.SQLiteStore), Box: ro.cfg.Secrets})
	id := createOwnArtifact(t, ro)
	created := doJSON(t, ro, http.MethodPost, "/api/connections", map[string]any{"name": "Personal", "recipe": "bearer", "origin": "https://api.example.test", "secret": "old-secret", "artifact_id": id})
	require.Equal(t, http.StatusCreated, created.Code, created.Body.String())
	var c connectors.Connection
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &c))
	grant := doJSON(t, ro, http.MethodPost, "/api/artifacts/"+id+"/connections", map[string]any{"connection_id": c.ID})
	require.Equal(t, http.StatusCreated, grant.Code, grant.Body.String())
	path := "/api/connections/" + c.ID + "/reconnect"
	rejected := doJSON(t, ro, http.MethodPost, path, map[string]any{"secret": "replacement", "origin": "https://attacker.example.test"})
	assert.Equal(t, http.StatusBadRequest, rejected.Code)
	token, err := registry.Issue(1, id, "")
	require.NoError(t, err)
	denied := doWithToken(t, ro, http.MethodPost, path, token.Token(), map[string]any{"secret": "agent-secret"})
	assert.Equal(t, http.StatusForbidden, denied.Code)
	repaired := doJSON(t, ro, http.MethodPost, path, map[string]any{"secret": "replacement"})
	require.Equal(t, http.StatusOK, repaired.Code, repaired.Body.String())
	var result connectors.Connection
	require.NoError(t, json.Unmarshal(repaired.Body.Bytes(), &result))
	assert.Equal(t, c.ID, result.ID)
	assert.Equal(t, c.Origins, result.Origins)
	grants, err := ro.cfg.Connections.Grants(context.Background(), 1, id)
	require.NoError(t, err)
	require.Len(t, grants, 1)
	assert.Equal(t, c.ID, grants[0].Connection.ID)
	rejected = doJSON(t, ro, http.MethodPost, "/api/connections", map[string]any{"name": "Personal", "recipe": "bearer", "origin": "https://api.example.test", "secret": "secret", "artifact_id": "https://evil.example.test/"})
	assert.Equal(t, http.StatusNotFound, rejected.Code)
}

func TestConnectionOAuthReturnsToBoundToolOnSuccessAndCancellation(t *testing.T) {
	ro := newTestRouter(t)
	provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"private-test-token","token_type":"bearer","expires_in":3600}`)
	}))
	defer provider.Close()
	ro.cfg.Connections = connectors.New(connectors.Config{Store: ro.cfg.Store.(*store.SQLiteStore), Box: ro.cfg.Secrets, OAuthEnabled: true, GitHub: connectors.OAuthProvider{ClientID: "client", ClientSecret: "secret", AuthURL: provider.URL + "/auth", TokenURL: provider.URL + "/token", APIOrigin: provider.URL}, GateFactory: func(origins []string) (*egress.Gate, error) { return egress.NewForTests(origins, egress.Limits{}) }})
	id := createOwnArtifact(t, ro)
	for _, cancelled := range []bool{false, true} {
		begin := doJSON(t, ro, http.MethodPost, "/api/connections/oauth/github/begin", map[string]any{"name": "Account", "artifact_id": id})
		require.Equal(t, http.StatusOK, begin.Code, begin.Body.String())
		var payload map[string]string
		require.NoError(t, json.Unmarshal(begin.Body.Bytes(), &payload))
		destination, err := url.Parse(payload["url"])
		require.NoError(t, err)
		query := url.Values{"state": {destination.Query().Get("state")}, "code": {"code"}, "artifact": {"https://evil.example.test/"}, "return_to": {"https://evil.example.test/"}}
		if cancelled {
			query.Del("code")
			query.Set("error", "access_denied")
		}
		req := httptest.NewRequest(http.MethodGet, "/api/connections/oauth/github/callback?"+query.Encode(), nil)
		for _, cookie := range begin.Result().Cookies() {
			req.AddCookie(cookie)
		}
		response := httptest.NewRecorder()
		ro.ServeHTTP(response, req)
		require.Equal(t, http.StatusSeeOther, response.Code, response.Body.String())
		target, err := url.Parse(response.Header().Get("Location"))
		require.NoError(t, err)
		assert.Empty(t, target.Host)
		assert.Equal(t, "/connections", target.Path)
		assert.Equal(t, id, target.Query().Get("artifact"))
		assert.NotContains(t, target.String(), "evil")
		if cancelled {
			assert.Equal(t, "cancelled", target.Query().Get("connection_error"))
		} else {
			assert.Equal(t, "github", target.Query().Get("connected"))
			assert.NotEmpty(t, target.Query().Get("connection"))
		}
	}
	bad := doJSON(t, ro, http.MethodPost, "/api/connections/oauth/github/begin", map[string]any{"artifact_id": "unknown"})
	assert.Equal(t, http.StatusNotFound, bad.Code)
}

func TestHostedConnectionOAuthKeepsAccountAndToolAcrossProviderReturn(t *testing.T) {
	ro, st := newLoginTestRouter(t, nil, nil, func(c *Config) { c.Clerk = hostedLoginStub{}; c.AuthToken = "" })
	ro.cfg.Connections = connectors.New(connectors.Config{Store: st.(*store.SQLiteStore), Box: ro.cfg.Secrets, OAuthEnabled: true, GitHub: connectors.OAuthProvider{ClientID: "client", ClientSecret: "secret"}})
	subject := "clerk:connected-owner"
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Test-Clerk-Subject", subject)
	owner, ok := ro.sessionUser(request)
	require.True(t, ok)
	require.NoError(t, st.PutArtifact(context.Background(), &store.Artifact{ID: "hosted-connected-tool", OwnerID: owner, Title: "My tool", SourceBlobID: "body", Tier: store.Tier1}))
	beginRequest := httptest.NewRequest(http.MethodPost, "/api/connections/oauth/github/begin", strings.NewReader(`{"name":"GitHub","artifact_id":"hosted-connected-tool"}`))
	beginRequest.Header.Set("Test-Clerk-Subject", subject)
	beginRequest.Header.Set("Content-Type", "application/json")
	begin := httptest.NewRecorder()
	ro.ServeHTTP(begin, beginRequest)
	require.Equal(t, http.StatusOK, begin.Code, begin.Body.String())
	var output map[string]string
	require.NoError(t, json.Unmarshal(begin.Body.Bytes(), &output))
	destination, err := url.Parse(output["url"])
	require.NoError(t, err)
	callback := httptest.NewRequest(http.MethodGet, "/api/connections/oauth/github/callback?error=access_denied&state="+url.QueryEscape(destination.Query().Get("state")), nil)
	callback.Header.Set("Test-Clerk-Subject", subject)
	for _, cookie := range begin.Result().Cookies() {
		callback.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	ro.ServeHTTP(response, callback)
	require.Equal(t, http.StatusSeeOther, response.Code, response.Body.String())
	assert.Contains(t, response.Header().Get("Location"), "artifact=hosted-connected-tool")
	assert.Contains(t, response.Header().Get("Location"), "connection_error=cancelled")
}
