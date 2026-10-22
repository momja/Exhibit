package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/momja/Exhibit/internal/connectors"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func navigationCall(t *testing.T, ro *Router, id, destination, policy string) *httptest.ResponseRecorder {
	t.Helper()
	if policy == "" {
		a, err := ro.cfg.Store.GetArtifact(context.Background(), 1, id)
		require.NoError(t, err)
		policy, err = ro.relayPolicy(context.Background(), 1, a)
		require.NoError(t, err)
	}
	body, err := json.Marshal(map[string]string{"url": destination})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/artifacts/"+id+"/navigation-check", strings.NewReader(string(body)))
	req.Header.Set("Authorization", authHeader())
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Exhibit-Relay-Policy", policy)
	w := httptest.NewRecorder()
	ro.ServeHTTP(w, req)
	return w
}

func TestNavigationCannotBypassConnectionDestinationsOrStaleHostPolicy(t *testing.T) {
	ro, registry := newScopedTestRouter(t)
	ctx := context.Background()
	service := connectors.New(connectors.Config{Store: ro.cfg.Store.(*store.SQLiteStore), Box: ro.cfg.Secrets})
	ro.cfg.Connections = service
	id := createOwnArtifact(t, ro)
	a, err := ro.cfg.Store.GetArtifact(ctx, 1, id)
	require.NoError(t, err)
	oldPolicy, err := ro.relayPolicy(ctx, 1, a)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, navigationCall(t, ro, id, "http://example.com/docs", "").Code)
	c, err := service.Create(ctx, 1, connectors.CreateRequest{Name: "Private account", Recipe: "bearer", Origin: "https://api.example.test", Secret: "private-account-key"}, connectors.UnlimitedLimits())
	require.NoError(t, err)
	_, err = service.Grant(ctx, 1, id, connectors.GrantRequest{ConnectionID: c.ID, PermittedEgress: []string{"https://reports.example.test"}})
	require.NoError(t, err)
	assert.Equal(t, http.StatusConflict, navigationCall(t, ro, id, "https://attacker.example/?data=private", oldPolicy).Code)
	for _, destination := range []string{"https://api.example.test/docs", "https://reports.example.test/result"} {
		w := navigationCall(t, ro, id, destination, "")
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.Contains(t, w.Body.String(), destination)
	}
	w := navigationCall(t, ro, id, "https://attacker.example/?data=private", "")
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	// Turning off the experiment must not discard the data restrictions.
	ro.cfg.ConnectionPolicy = service
	ro.cfg.Connections = nil
	assert.Equal(t, http.StatusForbidden, navigationCall(t, ro, id, "https://attacker.example/", "").Code)
	require.NoError(t, service.Ungrant(ctx, 1, id, c.ID))
	assert.Equal(t, http.StatusForbidden, navigationCall(t, ro, id, "https://attacker.example/", "").Code, "removing credential authority retains saved-data restrictions")
	require.NoError(t, service.Revoke(ctx, 1, c.ID))
	assert.Equal(t, http.StatusForbidden, navigationCall(t, ro, id, "https://attacker.example/", "").Code, "revoking the credential retains saved-data restrictions")
	assert.Equal(t, http.StatusOK, navigationCall(t, ro, id, "https://reports.example.test/result", "").Code)
	grant, err := registry.Issue(1, id, "")
	require.NoError(t, err)
	w = doWithToken(t, ro, http.MethodPost, "/api/artifacts/"+id+"/navigation-check", grant.Token(), map[string]string{"url": "https://api.example.test"})
	assert.Equal(t, http.StatusForbidden, w.Code, "agent grants cannot spend the host navigation authority")
}

func TestPublicNavigationExemptionIsOnlyAnonymousAndUnconnected(t *testing.T) {
	a := &store.Artifact{ID: "app-1", OwnerID: 1, Title: "Public app"}
	for _, test := range []struct {
		anonymous, protected, allowed bool
	}{{false, false, false}, {false, true, false}, {true, false, true}, {true, true, false}} {
		urls := testRenderURLs("https://render.example.test")
		urls.anonymous = test.anonymous
		creds := testPageCreds
		creds.ReadOnly = true
		page, err := renderDetailPage(a, urls, creds, nil, detailNetworkPolicy{Token: "policy", Protected: test.protected})
		require.NoError(t, err)
		if test.allowed {
			assert.Contains(t, page, "const PUBLIC_NAVIGATION = true;")
		} else {
			assert.Contains(t, page, "const PUBLIC_NAVIGATION = false;")
		}
	}
}

func TestUnconnectedPassThroughNavigationKeepsExistingLinkBehavior(t *testing.T) {
	ro := newTestRouter(t)
	id := createOwnArtifact(t, ro)
	ro.cfg.AuthToken = ""
	page := getPage(t, ro, "/artifacts/"+id)
	assert.Contains(t, page, "const PUBLIC_NAVIGATION = true;", "a pass-through instance has no credential for the viewer endpoint")
}

func TestStateFrameResyncRequiresCurrentPolicyBeforeReturningPrivateData(t *testing.T) {
	ro := newTestRouter(t)
	id := createOwnArtifact(t, ro)
	ctx := context.Background()
	a, err := ro.cfg.Store.GetArtifact(ctx, 1, id)
	require.NoError(t, err)
	oldPolicy, err := ro.relayPolicy(ctx, 1, a)
	require.NoError(t, err)
	service := connectors.New(connectors.Config{Store: ro.cfg.Store.(*store.SQLiteStore), Box: ro.cfg.Secrets})
	ro.cfg.ConnectionPolicy = service
	c, err := service.Create(ctx, 1, connectors.CreateRequest{Name: "Private account", Recipe: "bearer", Origin: "https://api.example.test", Secret: "secret"}, connectors.UnlimitedLimits())
	require.NoError(t, err)
	_, err = service.Grant(ctx, 1, id, connectors.GrantRequest{ConnectionID: c.ID})
	require.NoError(t, err)
	require.NoError(t, ro.cfg.Store.SetState(ctx, 1, id, 1, "balance", "PRIVATE-BALANCE-5932"))
	req := httptest.NewRequest(http.MethodGet, "/api/artifacts/"+id+"/state", nil)
	req.Header.Set("Authorization", authHeader())
	req.Header.Set("X-Exhibit-Relay-Policy", oldPolicy)
	w := httptest.NewRecorder()
	ro.ServeHTTP(w, req)
	assert.Equal(t, http.StatusConflict, w.Code)
	assert.NotContains(t, w.Body.String(), "PRIVATE-BALANCE-5932")
	w = getWithConnectionPolicy(t, ro, 1, id, "/api/artifacts/"+id+"/state", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "PRIVATE-BALANCE-5932")
	require.NoError(t, service.Revoke(ctx, 1, c.ID))
	req = httptest.NewRequest(http.MethodGet, "/api/artifacts/"+id+"/state", nil)
	req.Header.Set("Authorization", authHeader())
	req.Header.Set("X-Exhibit-Relay-Policy", oldPolicy)
	w = httptest.NewRecorder()
	ro.ServeHTTP(w, req)
	assert.Equal(t, http.StatusConflict, w.Code)
	assert.NotContains(t, w.Body.String(), "PRIVATE-BALANCE-5932", "old broad frames cannot receive retained private state after revocation")
}

func TestRevokedConnectionStillProtectsRenderNavigationAndSavedData(t *testing.T) {
	ro := newTestRouter(t)
	ctx := context.Background()
	id := createOwnArtifact(t, ro)
	for _, destination := range []string{"https://api.example.test", "https://attacker.example"} {
		require.NoError(t, ro.cfg.Store.SetOriginDecision(ctx, 1, id, destination, store.DecisionAllow, "test"))
	}
	service := connectors.New(connectors.Config{Store: ro.cfg.Store.(*store.SQLiteStore), Box: ro.cfg.Secrets})
	ro.cfg.ConnectionPolicy = service
	c, err := service.Create(ctx, 1, connectors.CreateRequest{Name: "Private account", Recipe: "bearer", Origin: "https://api.example.test", Secret: "secret"}, connectors.UnlimitedLimits())
	require.NoError(t, err)
	_, err = service.Grant(ctx, 1, id, connectors.GrantRequest{ConnectionID: c.ID})
	require.NoError(t, err)
	require.NoError(t, ro.cfg.Store.SetState(ctx, 1, id, 1, "balance", "PRIVATE-RETAINED-5923"))
	require.NoError(t, service.Revoke(ctx, 1, c.ID))
	protected, err := ro.hasConnectionDataPolicies(ctx, 1, id)
	require.NoError(t, err)
	require.True(t, protected)
	for _, framed := range []bool{false, true} {
		mintRequest := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(withPrincipal(ctx, Principal{OwnerID: 1, Kind: PrincipalServiceToken}))
		req := httptest.NewRequest(http.MethodGet, ro.renderURLs(mintRequest).artifact(id), nil)
		if framed {
			req.Header.Set("Sec-Fetch-Dest", "iframe")
		}
		w := httptest.NewRecorder()
		ro.RenderHandler().ServeHTTP(w, req)
		if framed {
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			assert.Contains(t, w.Body.String(), "PRIVATE-RETAINED-5923")
			assert.Contains(t, w.Header().Get("Content-Security-Policy"), "https://api.example.test")
			assert.NotContains(t, w.Header().Get("Content-Security-Policy"), "https://attacker.example")
		} else {
			assert.Equal(t, http.StatusForbidden, w.Code)
			assert.NotContains(t, w.Body.String(), "PRIVATE-RETAINED-5923")
		}
	}
	page := getPage(t, ro, "/artifacts/"+id)
	assert.Contains(t, page, `id="direct-open-toolbar" disabled`)
	assert.Contains(t, page, "const PUBLIC_NAVIGATION = false;")
}

func TestConnectedPrivateRenderRequiresTrustedFrameAndPublicShareIsExplicit(t *testing.T) {
	ro := newTestRouter(t)
	ctx := context.Background()
	id := createArtifact(t, ro, map[string]any{"title": "Private app", "body": "<h1>PRIVATE-SOURCE-5932</h1>"})
	service := connectors.New(connectors.Config{Store: ro.cfg.Store.(*store.SQLiteStore), Box: ro.cfg.Secrets})
	ro.cfg.ConnectionPolicy = service // grants remain enforced with every flag off
	c, err := service.Create(ctx, 1, connectors.CreateRequest{Name: "Account", Recipe: "bearer", Origin: "https://api.example.test", Secret: "private-key"}, connectors.UnlimitedLimits())
	require.NoError(t, err)
	_, err = service.Grant(ctx, 1, id, connectors.GrantRequest{ConnectionID: c.ID})
	require.NoError(t, err)
	require.NoError(t, ro.cfg.Store.SetState(ctx, 1, id, 1, "balance", "PRIVATE-BALANCE-5932"))
	for _, dest := range []string{"", "document", "empty", "iframe"} {
		mintRequest := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(withPrincipal(ctx, Principal{OwnerID: 1, Kind: PrincipalServiceToken}))
		req := httptest.NewRequest(http.MethodGet, ro.renderURLs(mintRequest).artifact(id), nil)
		if dest != "" {
			req.Header.Set("Sec-Fetch-Dest", dest)
		}
		w := httptest.NewRecorder()
		ro.RenderHandler().ServeHTTP(w, req)
		if dest == "iframe" {
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			assert.Contains(t, w.Body.String(), "PRIVATE-BALANCE-5932")
			assert.Contains(t, w.Header().Get("Content-Security-Policy"), "sandbox allow-scripts allow-forms")
		} else {
			assert.Equal(t, http.StatusForbidden, w.Code)
			assert.NotContains(t, w.Body.String(), "PRIVATE-BALANCE-5932")
			assert.NotContains(t, w.Body.String(), "PRIVATE-SOURCE-5932")
			assert.Contains(t, w.Body.String(), "/artifacts/"+id)
		}
	}
	page := doJSON(t, ro, http.MethodGet, "/artifacts/"+id, nil)
	require.Equal(t, http.StatusOK, page.Code, page.Body.String())
	assert.Contains(t, page.Header().Get("Content-Security-Policy"), "/a/"+id)
	assert.Contains(t, page.Body.String(), `id="direct-open-toolbar" disabled`)
	preview := doJSON(t, ro, http.MethodGet, "/agent?artifact="+id, nil)
	require.Equal(t, http.StatusOK, preview.Code, preview.Body.String())
	assert.Contains(t, preview.Body.String(), "data-relay-src=")
	assert.Contains(t, preview.Body.String(), `data-relay-host-target="framePolicy"`)
	_, err = service.Grant(ctx, 1, id, connectors.GrantRequest{ConnectionID: c.ID, ShareWithRecipients: true})
	require.NoError(t, err)
	w := doJSON(t, ro, http.MethodPost, "/api/shares", map[string]string{"artifact_id": id})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var share createShareResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &share))
	rendered := renderGet(t, ro, "/s/"+share.Share.ID)
	require.Equal(t, http.StatusOK, rendered.Code, rendered.Body.String())
	assert.Contains(t, rendered.Body.String(), "PRIVATE-BALANCE-5932", "the owner explicitly published these rows")
}
