package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/momja/Exhibit/internal/agentscope"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Owner-issued agent grants (Exh-l2xj). The acceptance criteria, in order: a
// minted token reaches its artifact and the sub-resources agentScopeAllows
// lists, and nothing else; revoking it makes the next request 401; another
// owner's artifact is a 404; and the token is stored only as a digest.

func doWithCookie(t *testing.T, ro *Router, method, path string, c *http.Cookie, body any) *httptest.ResponseRecorder {
	t.Helper()
	var rdr *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		rdr = bytes.NewReader(b)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, rdr)
	req.Header.Set("Content-Type", "application/json")
	if c != nil {
		req.AddCookie(c)
	}
	w := httptest.NewRecorder()
	ro.ServeHTTP(w, req)
	return w
}

// issueAgentGrant mints a grant over id with the service token, which a
// no-login test router accepts as the person at the page.
func issueAgentGrant(t *testing.T, r *Router, id, label string) (string, store.AgentGrant) {
	t.Helper()
	w := doJSON(t, r, "POST", "/api/artifacts/"+id+"/agent-grants", map[string]any{"label": label})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var resp struct {
		Grant store.AgentGrant `json:"grant"`
		Token string           `json:"token"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, agentscope.IsPersistedToken(resp.Token), resp.Token)
	return resp.Token, resp.Grant
}

func TestAgentGrantReachesItsArtifactAndRecordsAgentVersions(t *testing.T) {
	r := newTestRouter(t)
	id := createArtifact(t, r, map[string]any{"title": "A", "body": "<html><body>a</body></html>"})
	tok, grant := issueAgentGrant(t, r, id, "  Claude   Code\non my laptop ")
	assert.Equal(t, "Claude Code on my laptop", grant.Label, "a label is one line")

	w := doWithToken(t, r, "GET", "/api/artifacts/"+id+"?body=true", tok, nil)
	require.Equal(t, http.StatusOK, w.Code)

	// A write without the revision it read is refused, exactly as for Pi.
	w = doWithToken(t, r, "PATCH", "/api/artifacts/"+id, tok, map[string]any{"body": "<html><body>b</body></html>"})
	assert.Equal(t, http.StatusPreconditionRequired, w.Code)

	w = doWithToken(t, r, "PATCH", "/api/artifacts/"+id, tok, map[string]any{
		"body": "<html><body>b</body></html>", "expected_source_revision": contentRevisionForTest(t, r, id)})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	w = doWithToken(t, r, "PUT", "/api/artifacts/"+id+"/state", tok, map[string]any{"key": "k", "value": "v"})
	require.Equal(t, http.StatusNoContent, w.Code)
	w = doWithToken(t, r, "GET", "/api/artifacts/"+id+"/state", tok, nil)
	require.Equal(t, http.StatusOK, w.Code)
	w = doWithToken(t, r, "PUT", "/api/artifacts/"+id+"/widget", tok, map[string]any{
		"body": "<html><body>tile</body></html>", "expected_widget_revision": widgetSnapshot(t, r, id).WidgetRevision})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	w = doWithToken(t, r, "GET", "/api/artifacts/"+id+"/assets", tok, nil)
	require.Equal(t, http.StatusOK, w.Code)

	// The versions it wrote name the grant, from the row rather than the request.
	w = doJSON(t, r, "GET", "/api/artifacts/"+id+"/versions", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var history struct {
		Versions []store.Version `json:"versions"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &history))
	require.Len(t, history.Versions, 3)
	for _, v := range history.Versions[:2] {
		assert.Equal(t, store.VersionAgent, v.Origin)
		assert.Equal(t, "Claude Code on my laptop", v.Message)
		assert.Equal(t, "grant:"+grant.ID, v.SessionID)
	}
}

func TestAgentGrantReachesNothingButItsArtifact(t *testing.T) {
	r := newTestRouter(t)
	id := createArtifact(t, r, map[string]any{"title": "A", "body": "<html><body>a</body></html>"})
	other := createArtifact(t, r, map[string]any{"title": "B", "body": "<html><body>b</body></html>"})
	tok, grant := issueAgentGrant(t, r, id, "agent")

	refused := []struct{ method, path string }{
		{"POST", "/api/artifacts"}, // always bound: never the create
		{"GET", "/api/artifacts"},
		{"DELETE", "/api/artifacts/" + id},
		{"GET", "/api/artifacts/" + other},
		{"PATCH", "/api/artifacts/" + other},
		{"GET", "/api/artifacts/" + other + "/state"},
		{"PUT", "/api/artifacts/" + other + "/widget"},
		{"POST", "/api/artifacts/" + id + "/refetch"},
		{"POST", "/api/artifacts/" + id + "/duplicate"},
		{"GET", "/api/artifacts/" + id + "/versions"},
		{"POST", "/api/artifacts/" + id + "/versions/1/restore"},
		{"GET", "/api/artifacts/" + id + "/origins"},
		{"POST", "/api/artifacts/" + id + "/origins"},
		{"GET", "/api/artifacts/" + id + "/shares"},
		{"GET", "/api/artifacts/" + id + "/transcripts"},
		{"GET", "/api/artifacts/" + id + "/export"},
		{"POST", "/api/artifacts/" + id + "/state/import"},
		{"POST", "/api/artifacts/" + id + "/widget/generate"},
		{"DELETE", "/api/artifacts/" + id + "/assets/anything"},
		// A grant cannot extend itself, list its siblings or revoke them.
		{"GET", "/api/artifacts/" + id + "/agent-grants"},
		{"POST", "/api/artifacts/" + id + "/agent-grants"},
		{"DELETE", "/api/artifacts/" + id + "/agent-grants/" + grant.ID},
		{"POST", "/api/shares"},
		{"GET", "/api/tags"},
		{"GET", "/api/collections"},
		{"GET", "/api/agent/key"},
		{"POST", "/api/agent/sessions"},
		{"DELETE", "/api/account"},
		{"GET", "/api/admin/users"},
	}
	for _, c := range refused {
		w := doWithToken(t, r, c.method, c.path, tok, map[string]any{})
		assert.Equalf(t, http.StatusForbidden, w.Code, "%s %s should be refused", c.method, c.path)
	}
}

func TestAgentCredentialReportsItsOwnScopeOnly(t *testing.T) {
	r := newTestRouter(t)
	id := createArtifact(t, r, map[string]any{"title": "A", "body": "<html><body>a</body></html>"})
	tok, _ := issueAgentGrant(t, r, id, "agent")

	w := doWithToken(t, r, "GET", "/api/agent/credential", tok, nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"artifact_id":"`+id+`"}`, w.Body.String())

	w = doJSON(t, r, "GET", "/api/agent/credential", nil)
	assert.Equal(t, http.StatusNotFound, w.Code, "a person's credential has no agent scope")
	w = doWithToken(t, r, "POST", "/api/agent/credential", tok, nil)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestRevokedAgentGrantIsUnauthorized(t *testing.T) {
	r := newTestRouter(t)
	id := createArtifact(t, r, map[string]any{"title": "A", "body": "<html><body>a</body></html>"})
	tok, grant := issueAgentGrant(t, r, id, "agent")
	keep, _ := issueAgentGrant(t, r, id, "other agent")

	w := doJSON(t, r, "GET", "/api/artifacts/"+id+"/agent-grants", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var list struct {
		Grants []store.AgentGrant `json:"grants"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	require.Len(t, list.Grants, 2)
	assert.NotContains(t, w.Body.String(), tok, "a listing never carries a token")

	w = doJSON(t, r, "DELETE", "/api/artifacts/"+id+"/agent-grants/"+grant.ID, nil)
	require.Equal(t, http.StatusNoContent, w.Code)
	w = doJSON(t, r, "DELETE", "/api/artifacts/"+id+"/agent-grants/"+grant.ID, nil)
	assert.Equal(t, http.StatusNotFound, w.Code)

	w = doWithToken(t, r, "GET", "/api/artifacts/"+id, tok, nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	w = doWithToken(t, r, "GET", "/api/artifacts/"+id, keep, nil)
	assert.Equal(t, http.StatusOK, w.Code, "revoking one grant leaves the others")

	// Deleting the artifact takes its grants with it.
	w = doJSON(t, r, "DELETE", "/api/artifacts/"+id, nil)
	require.Equal(t, http.StatusNoContent, w.Code)
	w = doWithToken(t, r, "GET", "/api/artifacts/"+id, keep, nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAgentGrantTokenIsStoredOnlyAsADigest(t *testing.T) {
	r := newTestRouter(t)
	id := createArtifact(t, r, map[string]any{"title": "A", "body": "<html><body>a</body></html>"})
	tok, grant := issueAgentGrant(t, r, id, "agent")
	ctx := context.Background()

	byPlaintext, err := r.cfg.Store.ResolveAgentGrant(ctx, tok)
	require.NoError(t, err)
	assert.Nil(t, byPlaintext, "the plaintext is not what the store holds")
	byDigest, err := r.cfg.Store.ResolveAgentGrant(ctx, agentscope.PersistedTokenDigest(tok))
	require.NoError(t, err)
	require.NotNil(t, byDigest)
	assert.Equal(t, grant.ID, byDigest.ID)
}

// Only the owner, as a person, issues a grant. A recipient, another tenant
// and the operator's CLI token on an instance with a login are refused; the
// owner's own session is not.
func TestOnlyTheOwnersSessionIssuesAgentGrants(t *testing.T) {
	in := newTwoOwnerInstance(t)
	ctx := context.Background()
	cookieOne := sessionCookieFor(t, in.ro.cfg.Store, in.ownerOne, "session-owner-one")
	// Owner two is a recipient of owner one's artifact.
	_, err := in.ro.cfg.Store.CreateGrant(ctx, in.ownerOne, in.artifactOne, in.ownerTwo)
	require.NoError(t, err)

	path := "/api/artifacts/" + in.artifactOne + "/agent-grants"
	w := doWithCookie(t, in.ro, "POST", path, in.cookieTwo, map[string]any{"label": "x"})
	assert.Equal(t, http.StatusNotFound, w.Code, "a recipient gets a missing artifact")
	w = doWithCookie(t, in.ro, "GET", path, in.cookieTwo, nil)
	assert.Equal(t, http.StatusNotFound, w.Code)

	w = doWithToken(t, in.ro, "POST", path, "secret", map[string]any{"label": "x"})
	assert.Equal(t, http.StatusForbidden, w.Code, "the service token is not the owner's session on a login instance")

	w = doWithCookie(t, in.ro, "POST", path, cookieOne, map[string]any{"label": "x"})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var resp struct {
		Grant store.AgentGrant `json:"grant"`
		Token string           `json:"token"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	// The grant acts as owner one, on owner one's artifact only.
	w = doWithToken(t, in.ro, "GET", "/api/artifacts/"+in.artifactOne, resp.Token, nil)
	assert.Equal(t, http.StatusOK, w.Code)
	w = doWithToken(t, in.ro, "GET", "/api/artifacts/"+in.artifactTwo, resp.Token, nil)
	assert.Equal(t, http.StatusForbidden, w.Code)

	// The recipient cannot revoke it either.
	w = doWithCookie(t, in.ro, "DELETE", path+"/"+resp.Grant.ID, in.cookieTwo, nil)
	assert.Equal(t, http.StatusNotFound, w.Code)

	// Disabling the owner suspends the grant without deleting it.
	require.NoError(t, in.ro.cfg.Store.SetUserAdmin(ctx, in.ownerTwo, true), "so owner one is not the last admin")
	require.NoError(t, in.ro.cfg.Store.SetUserDisabled(ctx, in.ownerOne, true))
	w = doWithToken(t, in.ro, "GET", "/api/artifacts/"+in.artifactOne, resp.Token, nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAgentGrantsPanelIsTheOwners(t *testing.T) {
	r := newTestRouter(t)
	id := createArtifact(t, r, map[string]any{"title": "A", "body": "<html><body>a</body></html>"})
	_, _ = issueAgentGrant(t, r, id, "<b>Cursor</b>")

	w := doJSON(t, r, "GET", "/partials/agent-grants?artifact="+id, nil)
	require.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, "&lt;b&gt;Cursor&lt;/b&gt;", "a label is escaped")
	assert.Contains(t, body, `data-action="revoke-agent-grant"`)
	assert.Contains(t, body, `id="agent-grant-form"`)
	assert.Contains(t, body, "never used")

	w = doJSON(t, r, "GET", "/artifacts/"+id, nil)
	require.Equal(t, http.StatusOK, w.Code)
	page := w.Body.String()
	for _, id := range []string{"agent-grants-open", "agent-grants-panel", "agent-grants-body", "agent-grant-issued",
		"agent-grant-token", "agent-grant-copy", "agent-grants-status", "agent-grants-close", "agent-grant-label",
		"agent-grant-command", "agent-grant-command-copy", "agent-grant-config", "agent-grant-config-copy",
		"agent-grant-stdio", "agent-grant-stdio-copy"} {
		assert.Contains(t, page, `id="`+id+`"`)
	}
	assert.Contains(t, page, "gallery/agent-grants.js")
	assert.Contains(t, page, `data-app-origin="http://app.test"`, "the config names this instance")

	w = doJSON(t, r, "GET", "/partials/agent-grants?artifact=no-such-artifact", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestRecipientPageHasNoAgentGrantsPanel(t *testing.T) {
	in := newTwoOwnerInstance(t)
	_, err := in.ro.cfg.Store.CreateGrant(context.Background(), in.ownerOne, in.artifactOne, in.ownerTwo)
	require.NoError(t, err)
	w := in.get(t, "/artifacts/"+in.artifactOne, in.cookieTwo)
	require.Equal(t, http.StatusOK, w.Code)
	assert.NotContains(t, w.Body.String(), "agent-grants")
}
