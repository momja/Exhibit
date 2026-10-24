package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/momja/Exhibit/internal/connectors"
	"github.com/momja/Exhibit/internal/egress"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConnectionPermissionUpdatePreservesCredentialAndInvalidatesOldAuthority(t *testing.T) {
	var calls atomic.Int64
	provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		assert.Equal(t, "Bearer original-permission-secret", r.Header.Get("Authorization"))
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer provider.Close()
	ro := newTestRouter(t)
	ro.cfg.Relay = RelayConfig{Enabled: true, GateFactory: egress.NewForTests}
	ro.cfg.Connections = connectors.New(connectors.Config{Store: ro.cfg.Store.(*store.SQLiteStore), Box: ro.cfg.Secrets})
	ro.cfg.ConnectionPolicy = ro.cfg.Connections
	ctx := t.Context()
	c, err := ro.cfg.Connections.Create(ctx, 1, connectors.CreateRequest{
		Name: "Saved account", Recipe: "bearer", Origin: provider.URL, Secret: "original-permission-secret", DailyLimit: 10, CacheMinutes: 5,
	}, connectors.UnlimitedLimits())
	require.NoError(t, err)
	id := createOwnArtifact(t, ro)
	otherID := createOwnArtifact(t, ro)
	for _, artifactID := range []string{id, otherID} {
		require.NoError(t, ro.cfg.Store.SetOriginDecision(ctx, 1, artifactID, provider.URL, store.DecisionAllow, "test"))
		_, err = ro.cfg.Connections.Grant(ctx, 1, artifactID, connectors.GrantRequest{ConnectionID: c.ID})
		require.NoError(t, err)
	}
	s, _ := ro.relayStore()
	require.NoError(t, s.SetRelayGrant(ctx, 1, id, store.RelayGrant{Reads: true, Writes: true}))
	require.NoError(t, ro.cfg.Store.SetStateAsViewer(ctx, 1, id, "saved", "existing response"))
	before, err := ro.cfg.Store.(*store.SQLiteStore).GetConnection(ctx, 1, c.ID)
	require.NoError(t, err)
	grantsBefore, err := ro.cfg.Store.(*store.SQLiteStore).ListConnectionGrants(ctx, 1, id)
	require.NoError(t, err)
	oldAuthority, err := ro.relayPolicy(ctx, 1, mustArtifact(t, ro, id))
	require.NoError(t, err)
	oldOtherAuthority, err := ro.relayPolicy(ctx, 1, mustArtifact(t, ro, otherID))
	require.NoError(t, err)
	input := relayRequest{URL: provider.URL, Method: http.MethodPost}
	assert.Equal(t, http.StatusForbidden, relayCall(t, ro, id, input, "").Code)
	assert.Zero(t, calls.Load())

	path := "/api/connections/" + c.ID
	w := doJSON(t, ro, http.MethodPatch, path, map[string]bool{"allow_writes": true})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	assert.NotContains(t, w.Body.String(), "original-permission-secret")
	assert.NotContains(t, w.Body.String(), "encrypted_secret")
	var updated connectors.Connection
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &updated))
	assert.Equal(t, c.ID, updated.ID)
	assert.Contains(t, updated.Capabilities, "write")
	assert.Equal(t, 10, updated.DailyLimit)
	assert.Equal(t, 5, updated.CacheMinutes)
	otherAuthority, err := ro.relayPolicy(ctx, 1, mustArtifact(t, ro, otherID))
	require.NoError(t, err)
	assert.NotEqual(t, oldOtherAuthority, otherAuthority, "all artifacts using the edited connection renew their authority")
	assert.Equal(t, http.StatusConflict, relayCall(t, ro, id, input, oldAuthority).Code)
	w = relayCall(t, ro, id, input, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.EqualValues(t, 1, calls.Load(), "the existing encrypted credential works without reconnecting")
	authority, err := ro.relayPolicy(ctx, 1, mustArtifact(t, ro, id))
	require.NoError(t, err)
	w = doJSON(t, ro, http.MethodPatch, path, map[string]bool{"allow_writes": false})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, http.StatusConflict, relayCall(t, ro, id, input, authority).Code)
	assert.Equal(t, http.StatusForbidden, relayCall(t, ro, id, input, "").Code)
	assert.EqualValues(t, 1, calls.Load())
	after, err := ro.cfg.Store.(*store.SQLiteStore).GetConnection(ctx, 1, c.ID)
	require.NoError(t, err)
	assert.Equal(t, before.EncryptedSecret, after.EncryptedSecret)
	assert.Equal(t, before.Name, after.Name)
	assert.Equal(t, before.RecipeJSON, after.RecipeJSON, "round-trip restores the original policy")
	grantsAfter, err := ro.cfg.Store.(*store.SQLiteStore).ListConnectionGrants(ctx, 1, id)
	require.NoError(t, err)
	assert.Equal(t, grantsBefore, grantsAfter)
	state, err := ro.cfg.Store.GetStateAsViewer(ctx, 1, id)
	require.NoError(t, err)
	assert.Equal(t, "existing response", state["saved"])
}

func TestConnectionPermissionUpdateRequiresOwnerAndTrustedHuman(t *testing.T) {
	in := newTwoOwnerInstance(t)
	ro, ctx := in.ro, t.Context()
	c, err := ro.cfg.Connections.Create(ctx, in.ownerOne, connectors.CreateRequest{
		Name: "Private connection", Recipe: "bearer", Origin: "https://api.example.test", Secret: "permission-owner-secret",
	}, connectors.UnlimitedLimits())
	require.NoError(t, err)
	_, err = ro.cfg.Connections.Grant(ctx, in.ownerOne, in.artifactOne, connectors.GrantRequest{ConnectionID: c.ID, ShareWithRecipients: true})
	require.NoError(t, err)
	require.NoError(t, ro.cfg.Store.CreateShare(ctx, in.ownerOne, &store.Share{
		ID: "permission-share", ArtifactID: in.artifactOne, RecipientID: &in.ownerTwo,
	}))
	path := "/api/connections/" + c.ID
	request := func(target string, cookie *http.Cookie, headers map[string]string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPatch, target, strings.NewReader(`{"allow_writes":true}`))
		r.Header.Set("Content-Type", "application/json")
		if cookie != nil {
			r.AddCookie(cookie)
		} else {
			r.Header.Set("Authorization", authHeader())
		}
		for name, value := range headers {
			r.Header.Set(name, value)
		}
		w := httptest.NewRecorder()
		ro.ServeHTTP(w, r)
		return w
	}
	w := request(path, in.cookieTwo, nil)
	require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
	missing := request("/api/connections/absent", in.cookieTwo, nil)
	assert.Equal(t, missing.Body.String(), w.Body.String(), "a shared recipient cannot identify or edit the owner's connection")
	for _, headers := range []map[string]string{{"Origin": "https://attacker.test"}, {"Sec-Fetch-Site": "cross-site"}} {
		w = request(path, nil, headers)
		assert.Equal(t, http.StatusForbidden, w.Code)
	}
	w = request(path, sessionCookieFor(t, ro.cfg.Store, in.ownerOne, "permission-owner-session"), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	scoped, registry := newScopedTestRouter(t)
	scoped.cfg.Connections = ro.cfg.Connections
	grant, err := registry.Issue(1, in.artifactOne, "")
	require.NoError(t, err)
	w = doWithToken(t, scoped, http.MethodPatch, path, grant.Token(), map[string]bool{"allow_writes": false})
	assert.Equal(t, http.StatusForbidden, w.Code, "artifact-agent grants cannot change credential permissions")
}

func TestConnectionPermissionUpdateRejectsMissingOrExtraFields(t *testing.T) {
	ro := newTestRouter(t)
	ro.cfg.Connections = connectors.New(connectors.Config{Store: ro.cfg.Store.(*store.SQLiteStore), Box: ro.cfg.Secrets})
	c, err := ro.cfg.Connections.Create(t.Context(), 1, connectors.CreateRequest{
		Name: "Saved account", Recipe: "bearer", Origin: "https://api.example.test", Secret: "original-secret",
	}, connectors.UnlimitedLimits())
	require.NoError(t, err)
	for _, body := range []any{map[string]any{}, map[string]any{"allow_writes": nil}, map[string]any{"allow_writes": "true"}, map[string]any{"allow_writes": true, "secret": "replacement"}} {
		w := doJSON(t, ro, http.MethodPatch, "/api/connections/"+c.ID, body)
		assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	}
	list, err := ro.cfg.Connections.List(t.Context(), 1)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.NotContains(t, list[0].Capabilities, "write")
}
