package api

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/momja/Exhibit/internal/bundle"
	"github.com/momja/Exhibit/internal/connectors"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelayBundleRetainsDestinationPolicyWhenConnectorsAreDisabled(t *testing.T) {
	ro := newTestRouter(t)
	ro.cfg.Relay.Enabled = true
	ro.cfg.Connections = connectors.New(connectors.Config{Store: ro.cfg.Store.(*store.SQLiteStore), Box: ro.cfg.Secrets})
	id := createOwnArtifact(t, ro)
	ctx := context.Background()
	for _, origin := range []string{"https://api.example.test", "https://elsewhere.example.test"} {
		require.NoError(t, ro.cfg.Store.SetOriginDecision(ctx, 1, id, origin, store.DecisionAllow, "test"))
	}
	connection, err := ro.cfg.Connections.Create(ctx, 1, connectors.CreateRequest{Name: "Private API", Recipe: "bearer", Origin: "https://api.example.test", Secret: "do-not-export-99112"}, connectors.UnlimitedLimits())
	require.NoError(t, err)
	_, err = ro.cfg.Connections.Grant(ctx, 1, id, connectors.GrantRequest{ConnectionID: connection.ID})
	require.NoError(t, err)
	ro.cfg.ConnectionPolicy = ro.cfg.Connections
	ro.cfg.Connections = nil
	w := doJSON(t, ro, http.MethodGet, "/api/artifacts/"+id+"/relay-bundle", nil)
	require.Equal(t, 200, w.Code, w.Body.String())
	zr, err := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
	require.NoError(t, err)
	var manifest bundle.Manifest
	for _, file := range zr.File {
		reader, err := file.Open()
		require.NoError(t, err)
		body, err := io.ReadAll(reader)
		require.NoError(t, reader.Close())
		require.NoError(t, err)
		assert.NotContains(t, string(body), "do-not-export-99112")
		assert.NotContains(t, string(body), connection.ID)
		if strings.HasSuffix(file.Name, "/exhibit.json") {
			require.NoError(t, json.Unmarshal(body, &manifest))
		}
	}
	assert.Equal(t, []string{"https://api.example.test"}, manifest.NetworkAllowlist)
	require.Len(t, manifest.Connections, 1)
	assert.Equal(t, "Private API", manifest.Connections[0].Name)
	assert.Equal(t, "bearer", manifest.Connections[0].Kind)
}

func TestClearingConnectionDataIsAnExplicitHumanActionAndInvalidatesOldPages(t *testing.T) {
	ro, registry := newScopedTestRouter(t)
	ro.cfg.Relay.Enabled = true
	ro.cfg.Connections = connectors.New(connectors.Config{Store: ro.cfg.Store.(*store.SQLiteStore), Box: ro.cfg.Secrets})
	ro.cfg.ConnectionPolicy = ro.cfg.Connections
	id := createOwnArtifact(t, ro)
	ctx := context.Background()
	base := []string{"https://api.example.test", "https://elsewhere.example.test"}
	for _, origin := range base {
		require.NoError(t, ro.cfg.Store.SetOriginDecision(ctx, 1, id, origin, store.DecisionAllow, "test"))
	}
	connection, err := ro.cfg.Connections.Create(ctx, 1, connectors.CreateRequest{Name: "Private API", Recipe: "bearer", Origin: base[0], Secret: "do-not-export-12412"}, connectors.UnlimitedLimits())
	require.NoError(t, err)
	_, err = ro.cfg.Connections.Grant(ctx, 1, id, connectors.GrantRequest{ConnectionID: connection.ID})
	require.NoError(t, err)
	require.NoError(t, ro.cfg.Store.SetState(ctx, 1, id, 1, "cached", "private cached data"))
	require.NoError(t, ro.cfg.Connections.Revoke(ctx, 1, connection.ID))
	allowed, err := ro.connectionOrigins(ctx, 1, id, base)
	require.NoError(t, err)
	assert.Equal(t, base[:1], allowed, "disconnecting retains saved-data protection")
	a, err := ro.cfg.Store.GetArtifact(ctx, 1, id)
	require.NoError(t, err)
	oldPolicy, err := ro.relayPolicy(ctx, 1, a)
	require.NoError(t, err)
	path := "/api/artifacts/" + id + "/connection-data"
	agent, err := registry.Issue(1, id, "")
	require.NoError(t, err)
	w := doWithToken(t, ro, http.MethodDelete, path, agent.Token(), nil)
	assert.Equal(t, http.StatusForbidden, w.Code)
	state, err := ro.cfg.Store.GetStateAsViewer(ctx, 1, id)
	require.NoError(t, err)
	assert.Equal(t, "private cached data", state["cached"])
	// Disabling acquisition must leave both protection and the explicit
	// human cleanup action available.
	ro.cfg.Connections = nil
	w = doJSON(t, ro, http.MethodDelete, path, nil)
	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
	state, err = ro.cfg.Store.GetStateAsViewer(ctx, 1, id)
	require.NoError(t, err)
	assert.Empty(t, state)
	allowed, err = ro.connectionOrigins(ctx, 1, id, base)
	require.NoError(t, err)
	assert.Equal(t, base, allowed)
	w = relayCall(t, ro, id, relayRequest{URL: base[1]}, oldPolicy)
	assert.Equal(t, http.StatusConflict, w.Code, "an old host cannot fetch under released restrictions")
}

func TestConnectionAPIOnlyAcquiresSecretsInTrustedAppAndReturnsMetadata(t *testing.T) {
	ro, registry := newScopedTestRouter(t)
	ro.cfg.Relay.Enabled = true
	ro.cfg.Connections = connectors.New(connectors.Config{Store: ro.cfg.Store.(*store.SQLiteStore), Box: ro.cfg.Secrets})
	id := createOwnArtifact(t, ro)
	body := map[string]any{"name": "Weather account", "recipe": "api-key", "origin": "https://api.example.test", "header": "X-API-Key", "secret": "never-return-secret-6921"}
	grant, err := registry.Issue(1, id, "")
	require.NoError(t, err)
	w := doWithToken(t, ro, http.MethodPost, "/api/connections", grant.Token(), body)
	assert.Equal(t, http.StatusForbidden, w.Code)
	request := httptest.NewRequest(http.MethodPost, "/api/connections", strings.NewReader(`{"name":"Weather","recipe":"bearer","origin":"https://api.example.test","secret":"x"}`))
	request.Header.Set("Authorization", authHeader())
	request.Header.Set("Origin", "https://hostile.example")
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	ro.ServeHTTP(response, request)
	assert.Equal(t, http.StatusForbidden, response.Code)
	w = doJSON(t, ro, http.MethodPost, "/api/connections", body)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	assert.NotContains(t, w.Body.String(), "never-return-secret-6921")
	assert.NotContains(t, w.Body.String(), "encrypted_secret")
	var connection connectors.Connection
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &connection))
	require.NotEmpty(t, connection.ID)
	w = doJSON(t, ro, http.MethodGet, "/api/connections", nil)
	require.Equal(t, 200, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "Weather account")
	assert.NotContains(t, w.Body.String(), "never-return-secret-6921")
	assert.NotContains(t, w.Body.String(), "recipe_json")
	w = doJSON(t, ro, http.MethodPost, "/api/artifacts/"+id+"/connections", map[string]any{"connection_id": connection.ID, "permitted_egress": []string{}})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "nowhere else")
	w = getWithConnectionPolicy(t, ro, 1, id, "/api/artifacts/"+id+"/connections", nil)
	require.Equal(t, 200, w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), "never-return-secret-6921")
	w = doJSON(t, ro, http.MethodDelete, "/api/connections/"+connection.ID, nil)
	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
	list, err := ro.cfg.Connections.Grants(context.Background(), 1, id)
	require.NoError(t, err)
	assert.Empty(t, list)
}

func TestBasicCredentialURLStaysOutOfMetadataAndExport(t *testing.T) {
	ro := newTestRouter(t)
	ro.cfg.Relay.Enabled = true
	ro.cfg.Connections = connectors.New(connectors.Config{Store: ro.cfg.Store.(*store.SQLiteStore), Box: ro.cfg.Secrets})
	id := createOwnArtifact(t, ro)
	ctx := context.Background()
	const origin = "https://api.example.test"
	require.NoError(t, ro.cfg.Store.SetOriginDecision(ctx, 1, id, origin, store.DecisionAllow, "test"))
	w := doJSON(t, ro, http.MethodPost, "/api/connections", map[string]any{
		"name": "Private Basic API", "recipe": "basic",
		"access_url":  "https://private-user-193:private-password-193@api.example.test/private-base-193/",
		"daily_limit": 2, "cache_minutes": 60,
	})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var connection connectors.Connection
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &connection))
	assert.Equal(t, []string{origin}, connection.Origins)
	assert.Equal(t, 2, connection.DailyLimit)
	assert.Equal(t, 60, connection.CacheMinutes)
	for _, secret := range []string{"private-user-193", "private-password-193", "private-base-193"} {
		assert.NotContains(t, w.Body.String(), secret)
	}
	w = doJSON(t, ro, http.MethodGet, "/api/connections", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	for _, secret := range []string{"private-user-193", "private-password-193", "private-base-193"} {
		assert.NotContains(t, w.Body.String(), secret)
	}
	w = doJSON(t, ro, http.MethodPost, "/api/artifacts/"+id+"/connections", map[string]any{"connection_id": connection.ID})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "nowhere else")
	w = doJSON(t, ro, http.MethodGet, "/api/artifacts/"+id+"/relay-bundle", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	zr, err := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
	require.NoError(t, err)
	var manifest bundle.Manifest
	for _, file := range zr.File {
		reader, err := file.Open()
		require.NoError(t, err)
		body, err := io.ReadAll(reader)
		require.NoError(t, reader.Close())
		require.NoError(t, err)
		for _, secret := range []string{"private-user-193", "private-password-193", "private-base-193", connection.ID} {
			assert.NotContains(t, string(body), secret, file.Name)
		}
		if strings.HasSuffix(file.Name, "/exhibit.json") {
			require.NoError(t, json.Unmarshal(body, &manifest))
		}
	}
	require.Len(t, manifest.Connections, 1)
	assert.Equal(t, "basic", manifest.Connections[0].Kind)
	assert.Equal(t, 2, manifest.Connections[0].DailyLimit)
	assert.Equal(t, 60, manifest.Connections[0].CacheMinutes)
	assert.Equal(t, []string{origin}, manifest.Connections[0].DataOrigins)
}

func TestManagedProviderIDExportsAsLocalBearerRequirement(t *testing.T) {
	ro := newTestRouter(t)
	ro.cfg.Relay.Enabled = true
	s := ro.cfg.Store.(*store.SQLiteStore)
	ro.cfg.Connections = connectors.New(connectors.Config{Store: s, Box: ro.cfg.Secrets})
	ctx := context.Background()
	id := createOwnArtifact(t, ro)
	const origin = "https://api.example.test"
	require.NoError(t, s.SetOriginDecision(ctx, 1, id, origin, store.DecisionAllow, "test"))
	sealed, err := ro.cfg.Secrets.Encrypt(`{"vendor_connection_id":"ca_private_fixture"}`)
	require.NoError(t, err)
	require.NoError(t, s.CreateConnection(ctx, &store.ConnectionRecord{
		ID: "managed-export", OwnerID: 1, Name: "Managed API", Provider: "notes-cloud", Kind: "vendor", Engine: "composio", ExternalRef: "ca_private_fixture",
		RecipeJSON: `{"origins":["https://api.example.test"],"vendor_provider":"notes-cloud","vendor_spec":{"engine":"composio","delivery_mode":"forward","origins":["https://api.example.test"]}}`, EncryptedSecret: sealed,
	}, -1))
	_, err = ro.cfg.Connections.Grant(ctx, 1, id, connectors.GrantRequest{ConnectionID: "managed-export"})
	require.NoError(t, err)
	w := doJSON(t, ro, http.MethodGet, "/api/artifacts/"+id+"/relay-bundle", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	zr, err := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
	require.NoError(t, err)
	var manifest bundle.Manifest
	for _, file := range zr.File {
		reader, err := file.Open()
		require.NoError(t, err)
		body, err := io.ReadAll(reader)
		require.NoError(t, reader.Close())
		require.NoError(t, err)
		assert.NotContains(t, string(body), "ca_private_fixture")
		assert.NotContains(t, string(body), "managed-export")
		if strings.HasSuffix(file.Name, "/exhibit.json") {
			require.NoError(t, json.Unmarshal(body, &manifest))
		}
	}
	require.Len(t, manifest.Connections, 1)
	assert.Equal(t, "bearer", manifest.Connections[0].Kind)
}

func getWithConnectionPolicy(t *testing.T, ro *Router, viewer int64, id, path string, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if cookie != nil {
		req.AddCookie(cookie)
	} else {
		req.Header.Set("Authorization", authHeader())
	}
	a, err := ro.cfg.Store.GetArtifactReadableBy(context.Background(), store.ViewerID(viewer), id)
	require.NoError(t, err)
	if a != nil {
		policy, err := ro.relayPolicy(context.Background(), viewer, a)
		require.NoError(t, err)
		req.Header.Set("X-Exhibit-Relay-Policy", policy)
	}
	w := httptest.NewRecorder()
	ro.ServeHTTP(w, req)
	return w
}

func TestConnectionSurfacesFollowTheSessionViewer(t *testing.T) {
	in := newTwoOwnerInstance(t)
	for _, who := range []struct {
		owner int64
		name  string
	}{{in.ownerOne, "First viewer API"}, {in.ownerTwo, "Second viewer API"}} {
		_, err := in.ro.cfg.Connections.Create(context.Background(), who.owner, connectors.CreateRequest{Name: who.name, Recipe: "bearer", Origin: "https://api.example.test", Secret: "scoped-secret"}, connectors.UnlimitedLimits())
		require.NoError(t, err)
	}
	w := in.get(t, "/api/connections", in.cookieTwo)
	require.Equal(t, 200, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "Second viewer API")
	assert.NotContains(t, w.Body.String(), "First viewer API")
	assert.NotContains(t, w.Body.String(), "scoped-secret")
	for _, suffix := range []string{"connections", "agent-connections", "relay-grant", "relay-schedules", "relay-bundle"} {
		w = getWithConnectionPolicy(t, in.ro, in.ownerTwo, in.artifactTwo, "/api/artifacts/"+in.artifactTwo+"/"+suffix, in.cookieTwo)
		require.Equal(t, 200, w.Code, suffix+": "+w.Body.String())
		w = getWithConnectionPolicy(t, in.ro, in.ownerTwo, in.artifactOne, "/api/artifacts/"+in.artifactOne+"/"+suffix, in.cookieTwo)
		assert.Equal(t, 404, w.Code, suffix+": "+w.Body.String())
		assert.NotContains(t, w.Body.String(), ownerOneTitle)
	}
}

func TestConnectionOAuthCallbackRejectsForgedNavigation(t *testing.T) {
	in := newProfileInstance(t, "connection-oauth-user", "oauth@example.test")
	in.ro.cfg.Relay.Enabled = true
	in.ro.cfg.Connections = connectors.New(connectors.Config{Store: in.st.(*store.SQLiteStore), Box: in.ro.cfg.Secrets, OAuthEnabled: true, GitHub: connectors.OAuthProvider{ClientID: "operator-client", ClientSecret: "operator-secret"}})
	req := httptest.NewRequest(http.MethodPost, "/api/connections/oauth/github/begin", strings.NewReader(`{"name":"GitHub"}`))
	req.AddCookie(in.cookie)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	in.ro.ServeHTTP(w, req)
	require.Equal(t, 200, w.Code, w.Body.String())
	var output struct {
		URL string `json:"url"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &output))
	destination, err := url.Parse(output.URL)
	require.NoError(t, err)
	require.NotEmpty(t, destination.Query().Get("state"))
	var flow *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == connectionFlowCookie {
			flow = c
		}
	}
	require.NotNil(t, flow)
	assert.True(t, flow.HttpOnly)
	for _, hasFlow := range []bool{false, true} {
		req = httptest.NewRequest(http.MethodGet, "/api/connections/oauth/github/callback?state=forged&code=attacker-code", nil)
		req.AddCookie(in.cookie)
		if hasFlow {
			req.AddCookie(flow)
		}
		w = httptest.NewRecorder()
		in.ro.ServeHTTP(w, req)
		assert.Equal(t, 400, w.Code, w.Body.String())
	}
	connections, err := in.ro.cfg.Connections.List(context.Background(), in.user.ID)
	require.NoError(t, err)
	assert.Empty(t, connections)
}

func TestPrivateConnectorCannotBePublishedThroughASharingChange(t *testing.T) {
	ro := newTestRouter(t)
	ro.cfg.Relay.Enabled = true
	ro.cfg.Connections = connectors.New(connectors.Config{Store: ro.cfg.Store.(*store.SQLiteStore), Box: ro.cfg.Secrets})
	id := createOwnArtifact(t, ro)
	c, err := ro.cfg.Connections.Create(context.Background(), 1, connectors.CreateRequest{Name: "Private bank", Recipe: "bearer", Origin: "https://api.example.test", Secret: "private-key"}, connectors.UnlimitedLimits())
	require.NoError(t, err)
	_, err = ro.cfg.Connections.Grant(context.Background(), 1, id, connectors.GrantRequest{ConnectionID: c.ID})
	require.NoError(t, err)
	w := doJSON(t, ro, http.MethodPost, "/api/shares", map[string]any{"artifact_id": id})
	assert.Equal(t, 409, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "explicitly enable sharing")
	w = doJSON(t, ro, http.MethodPatch, "/api/artifacts/"+id, map[string]any{"share_state_mode": store.ShareStateShared})
	assert.Equal(t, 409, w.Code, w.Body.String())
	_, err = ro.cfg.Connections.Grant(context.Background(), 1, id, connectors.GrantRequest{ConnectionID: c.ID, ShareWithRecipients: true})
	require.NoError(t, err)
	w = doJSON(t, ro, http.MethodPost, "/api/shares", map[string]any{"artifact_id": id})
	require.Equal(t, 201, w.Code, w.Body.String())
	w = doJSON(t, ro, http.MethodPatch, "/api/artifacts/"+id, map[string]any{"share_state_mode": store.ShareStateShared})
	require.Equal(t, 200, w.Code, w.Body.String())
}

func TestConnectionAPICannotUseAnotherViewersSecretOrRaiseItsLimit(t *testing.T) {
	ro := newTestRouter(t)
	ro.cfg.Relay.Enabled = true
	ro.cfg.Connections = connectors.New(connectors.Config{Store: ro.cfg.Store.(*store.SQLiteStore), Box: ro.cfg.Secrets})
	ro.cfg.Entitlements = Entitlements{Enforced: true, DefaultStorageBytes: 1 << 30, DefaultConnections: connectors.Limits{Native: 1, OAuth: 0, Vendor: 0}}
	foreign, err := ro.cfg.Connections.Create(context.Background(), 2, connectors.CreateRequest{Name: "Other account", Recipe: "bearer", Origin: "https://api.example.test", Secret: "foreign-secret-2748"}, connectors.UnlimitedLimits())
	require.NoError(t, err)
	w := doJSON(t, ro, http.MethodDelete, "/api/connections/"+foreign.ID, nil)
	assert.Equal(t, 404, w.Code)
	own := createOwnArtifact(t, ro)
	w = doJSON(t, ro, http.MethodPost, "/api/artifacts/"+own+"/connections", map[string]any{"connection_id": foreign.ID})
	assert.Equal(t, 404, w.Code)
	body := map[string]any{"name": "Allowed account", "recipe": "bearer", "origin": "https://api.example.test", "secret": "native-secret-9997", "connection_native_limit": 100}
	w = doJSON(t, ro, http.MethodPost, "/api/connections", body)
	require.Equal(t, 201, w.Code, w.Body.String())
	w = doJSON(t, ro, http.MethodPost, "/api/connections", body)
	require.Equal(t, 429, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "1 native connections")
	assert.NotContains(t, w.Body.String(), "native-secret-9997")
	w = doJSON(t, ro, http.MethodGet, "/api/connections", nil)
	require.Equal(t, 200, w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), "Other account")
	assert.NotContains(t, w.Body.String(), "foreign-secret-2748")
}
