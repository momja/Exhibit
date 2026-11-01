package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/momja/Exhibit/internal/agentscope"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func doWithCookie(t *testing.T, ro *Router, method, path string, cookie *http.Cookie, body any) *httptest.ResponseRecorder {
	t.Helper()
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		require.NoError(t, err)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		req.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	ro.ServeHTTP(w, req)
	return w
}

type issuedAgentGrant struct {
	Grant store.AgentGrant `json:"grant"`
	Token string           `json:"token"`
}

func decodeIssuedAgentGrant(t *testing.T, w *httptest.ResponseRecorder) issuedAgentGrant {
	t.Helper()
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var issued issuedAgentGrant
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &issued))
	require.True(t, agentscope.IsPersistedToken(issued.Token))
	require.NotEmpty(t, issued.Grant.ID)
	return issued
}

func issueAgentGrant(t *testing.T, r *Router, label string) issuedAgentGrant {
	t.Helper()
	return decodeIssuedAgentGrant(t, doJSON(t, r, "POST", "/api/agent-grants", map[string]any{"label": label}))
}

// A library credential can edit several existing artifacts and create several
// more. Creating must never bind it to the new artifact as a Pi grant would.
func TestLibraryGrantReachesOwnedArtifactsAndRecordsAgentVersions(t *testing.T) {
	r := newTestRouter(t)
	r.cfg.MCPURL = "https://agents.example.test/mcp"
	first := createArtifact(t, r, map[string]any{"title": "Water tracker", "body": "<p>water</p>"})
	second := createArtifact(t, r, map[string]any{"title": "Run logger", "body": "<p>run</p>"})
	// Labels are trimmed on issue, but internal whitespace remains part of
	// the owner's label and must survive unchanged in every version message.
	issued := issueAgentGrant(t, r, "  Claude  Code\non my laptop  ")
	require.Equal(t, "Claude  Code\non my laptop", issued.Grant.Label)
	for _, id := range []string{first, second} {
		path := "/api/artifacts/" + id
		w := doWithToken(t, r, "GET", path+"?body=true", issued.Token, nil)
		require.Equal(t, 200, w.Code, w.Body.String())
		w = doWithToken(t, r, "PATCH", path, issued.Token, map[string]any{"body": "<p>changed</p>"})
		require.Equal(t, http.StatusPreconditionRequired, w.Code)
		w = doWithToken(t, r, "PATCH", path, issued.Token, map[string]any{"body": "<p>changed</p>", "expected_source_revision": contentRevisionForTest(t, r, id)})
		require.Equal(t, 200, w.Code, w.Body.String())
		w = doJSON(t, r, "PUT", path+"/state", map[string]any{"key": "count", "value": "7"})
		require.Equal(t, 204, w.Code, w.Body.String())
		w = doWithToken(t, r, "GET", path+"/state", issued.Token, nil)
		require.Equal(t, 200, w.Code)
		assert.JSONEq(t, `{"count":"7"}`, w.Body.String())
		w = doWithToken(t, r, "PUT", path+"/widget", issued.Token, map[string]any{"body": "<p>tile</p>", "expected_widget_revision": widgetSnapshot(t, r, id).WidgetRevision})
		require.Equal(t, 200, w.Code, w.Body.String())
		w = doWithToken(t, r, "GET", path+"/widget", issued.Token, nil)
		require.Equal(t, 200, w.Code)
		assert.Contains(t, w.Body.String(), "tile")
		require.Equal(t, 200, doWithToken(t, r, "GET", path+"/assets", issued.Token, nil).Code)
		w = doJSON(t, r, "GET", path+"/versions", nil)
		require.Equal(t, 200, w.Code)
		var history struct {
			Versions []store.Version `json:"versions"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &history))
		require.Len(t, history.Versions, 3)
		for _, version := range history.Versions[:2] {
			assert.Equal(t, store.VersionAgent, version.Origin)
			assert.Equal(t, issued.Grant.Label, version.Message)
			assert.Equal(t, "grant:"+issued.Grant.ID, version.SessionID)
		}
		req := httptest.NewRequest("DELETE", path+"/widget", nil)
		req.Header.Set("Authorization", "Bearer "+issued.Token)
		req.Header.Set("If-Match", widgetSnapshot(t, r, id).WidgetRevision)
		w = httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.Equal(t, 204, w.Code, w.Body.String())
	}
	for range 2 {
		w := doWithToken(t, r, "POST", "/api/artifacts", issued.Token, map[string]any{"title": "Agent created", "body": "<p>new</p>"})
		require.Equal(t, 201, w.Code, w.Body.String())
		var created struct {
			Artifact store.Artifact `json:"artifact"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
		require.NotEmpty(t, created.Artifact.ID)
		versions, err := r.cfg.Store.ListVersions(context.Background(), defaultOwnerID, created.Artifact.ID)
		require.NoError(t, err)
		require.Len(t, versions, 1)
		assert.Equal(t, store.VersionAgent, versions[0].Origin)
		assert.Equal(t, issued.Grant.Label, versions[0].Message)
		assert.Equal(t, "grant:"+issued.Grant.ID, versions[0].SessionID)
	}
	require.Equal(t, 200, doWithToken(t, r, "GET", "/api/artifacts/"+first, issued.Token, nil).Code)
	credential := doWithToken(t, r, "GET", "/api/agent/credential", issued.Token, nil)
	require.Equal(t, 200, credential.Code)
	assert.JSONEq(t, `{"scope":"library","grant_id":"`+issued.Grant.ID+`","label":"Claude  Code\non my laptop"}`, credential.Body.String())
}

func TestLibraryGrantListsSearchesAndCreatesOnlyForItsOwner(t *testing.T) {
	in := newTwoOwnerInstance(t)
	in.ro.cfg.MCPURL = "https://agents.example.test/mcp"
	// Owner two can view owner one's artifact as a person, but its agent cannot.
	_, err := in.ro.cfg.Store.CreateGrant(context.Background(), in.ownerOne, in.artifactOne, in.ownerTwo)
	require.NoError(t, err)
	issued := decodeIssuedAgentGrant(t, doWithCookie(t, in.ro, "POST", "/api/agent-grants", in.cookieTwo, map[string]string{"label": "Second owner"}))
	for _, query := range []string{"", "?q=" + ownerTwoTitle, "?q=" + ownerOneTitle} {
		w := doWithToken(t, in.ro, "GET", "/api/artifacts"+query, issued.Token, nil)
		require.Equal(t, 200, w.Code, w.Body.String())
		assert.NotContains(t, w.Body.String(), in.artifactOne)
		if query != "?q="+ownerOneTitle {
			assert.Contains(t, w.Body.String(), in.artifactTwo)
		}
	}
	w := doWithToken(t, in.ro, "POST", "/api/artifacts", issued.Token, map[string]string{"title": "New owner two artifact", "body": "<p>hello</p>"})
	require.Equal(t, 201, w.Code, w.Body.String())
	var created struct {
		Artifact store.Artifact `json:"artifact"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	artifact, err := in.ro.cfg.Store.GetArtifact(context.Background(), in.ownerTwo, created.Artifact.ID)
	require.NoError(t, err)
	require.NotNil(t, artifact)
	assert.Equal(t, in.ownerTwo, artifact.OwnerID)
}

// Every allowed per-artifact operation must distinguish ownership from viewing.
// Shared state and widgets are seeded, so a mistaken viewer lookup cannot pass
// by returning a missing-resource 404 for an otherwise accessible artifact.
func TestLibraryGrantRefusesSharedAndForeignArtifacts(t *testing.T) {
	in := newTwoOwnerInstance(t)
	in.ro.cfg.MCPURL = "https://agents.example.test/mcp"
	ctx := context.Background()
	sharedID := seedOwnedArtifact(t, in.ro, in.ownerOne, "shared-with-two", "Shared", "private-body", "private-state")
	_, err := in.ro.cfg.Store.CreateGrant(ctx, in.ownerOne, sharedID, in.ownerTwo)
	require.NoError(t, err)
	issued := decodeIssuedAgentGrant(t, doWithCookie(t, in.ro, "POST", "/api/agent-grants", in.cookieTwo, map[string]string{"label": "agent"}))
	cases := []struct {
		method, suffix string
		body           any
	}{
		{"GET", "?body=true", nil}, {"PATCH", "", map[string]string{"title": "hijacked"}},
		{"GET", "/state", nil},
		{"GET", "/widget", nil}, {"PUT", "/widget", map[string]string{"body": "<p>hijacked</p>", "expected_widget_revision": "fake"}},
		{"DELETE", "/widget", nil}, {"GET", "/assets", nil},
	}
	for _, id := range []string{sharedID, in.artifactOne, "missing"} {
		for _, tc := range cases {
			t.Run(id+tc.method+tc.suffix, func(t *testing.T) {
				w := doWithToken(t, in.ro, tc.method, "/api/artifacts/"+id+tc.suffix, issued.Token, tc.body)
				assert.Equal(t, 404, w.Code, w.Body.String())
				assert.NotContains(t, w.Body.String(), "private-")
			})
		}
	}
	state, err := in.ro.cfg.Store.GetState(ctx, store.OwnerID(in.ownerOne), sharedID, store.ViewerID(in.ownerOne))
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"note": "private-state"}, state)
}

func TestLibraryGrantRefusesOtherRouteFamilies(t *testing.T) {
	r := newTestRouter(t)
	r.cfg.MCPURL = "https://agents.example.test/mcp"
	id := createArtifact(t, r, map[string]any{"title": "A", "body": "<p>a</p>"})
	issued := issueAgentGrant(t, r, "agent")
	cases := []struct{ method, path string }{
		{"DELETE", "/api/artifacts/" + id}, {"POST", "/api/artifacts/" + id + "/duplicate"}, {"POST", "/api/artifacts/" + id + "/refetch"},
		{"GET", "/api/artifacts/" + id + "/versions"}, {"POST", "/api/artifacts/" + id + "/versions/1/restore"},
		{"GET", "/api/artifacts/" + id + "/origins"}, {"POST", "/api/artifacts/" + id + "/origins"}, {"DELETE", "/api/artifacts/" + id + "/origins"},
		{"GET", "/api/artifacts/" + id + "/shares"}, {"DELETE", "/api/artifacts/" + id + "/grants?recipient=2"},
		{"POST", "/api/shares"}, {"DELETE", "/api/shares/missing"},
		{"GET", "/api/artifacts/" + id + "/transcripts"}, {"GET", "/api/artifacts/" + id + "/export"},
		{"POST", "/api/artifacts/" + id + "/state/import"}, {"POST", "/api/artifacts/" + id + "/assets/import"}, {"GET", "/api/artifacts/" + id + "/assets/export"},
		{"DELETE", "/api/artifacts/" + id + "/assets/missing"}, {"POST", "/api/artifacts/" + id + "/widget/generate"},
		// Library access does not delegate personal connected accounts, relay
		// authority, schedules, or the Pi-only connection request workflow.
		{"GET", "/api/connections"}, {"POST", "/api/connections"}, {"DELETE", "/api/connections/missing"},
		{"POST", "/api/connections/missing/reconnect"}, {"POST", "/api/connections/oauth/github/begin"},
		{"POST", "/api/connections/vendor/begin"}, {"POST", "/api/connections/vendor/complete"},
		{"GET", "/api/artifacts/" + id + "/connections"}, {"POST", "/api/artifacts/" + id + "/connections"},
		{"DELETE", "/api/artifacts/" + id + "/connections/missing"}, {"POST", "/api/artifacts/" + id + "/connections/missing/standin"},
		{"DELETE", "/api/artifacts/" + id + "/connection-data"}, {"DELETE", "/api/artifacts/" + id + "/connection-requests/missing"},
		{"GET", "/api/artifacts/" + id + "/agent-connections"}, {"POST", "/api/artifacts/" + id + "/agent-connections"},
		{"POST", "/api/artifacts/" + id + "/relay"}, {"GET", "/api/artifacts/" + id + "/relay-bundle"},
		{"GET", "/api/artifacts/" + id + "/relay-grant"}, {"PUT", "/api/artifacts/" + id + "/relay-grant"},
		{"GET", "/api/artifacts/" + id + "/relay-schedules"}, {"POST", "/api/artifacts/" + id + "/relay-schedules"},
		{"DELETE", "/api/artifacts/" + id + "/relay-schedules/missing"},
		{"GET", "/connections"}, {"GET", "/partials/connections"},
		{"GET", "/api/agent-grants"}, {"POST", "/api/agent-grants"}, {"DELETE", "/api/agent-grants/" + issued.Grant.ID},
		{"GET", "/api/tags"}, {"POST", "/api/artifacts/" + id + "/tags/1"}, {"GET", "/api/collections"}, {"POST", "/api/artifacts/" + id + "/collections/1"},
		{"GET", "/api/agent/key"}, {"POST", "/api/agent/sessions"}, {"DELETE", "/api/account"}, {"GET", "/api/admin/users"},
		{"GET", "/"}, {"GET", "/settings"}, {"GET", "/profile"}, {"GET", "/artifacts/" + id}, {"GET", "/partials/agent-grants"}, {"GET", "/partials/gallery-grid"},
	}
	for _, tc := range cases {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			w := doWithToken(t, r, tc.method, tc.path, issued.Token, map[string]any{})
			assert.Equal(t, 403, w.Code, w.Body.String())
		})
	}
	w := doWithToken(t, r, "PATCH", "/api/artifacts/"+id, issued.Token, map[string]string{"share_state_mode": "shared", "title": "hijacked"})
	require.Equal(t, 400, w.Code, w.Body.String())
	a, err := r.cfg.Store.GetArtifact(context.Background(), defaultOwnerID, id)
	require.NoError(t, err)
	assert.Equal(t, "A", a.Title)
	assert.Equal(t, "own", a.ShareStateMode)
}

func TestAgentGrantRevocationAndDigestOnlyStorage(t *testing.T) {
	r := newTestRouter(t)
	r.cfg.MCPURL = "https://agents.example.test/mcp"
	issued := issueAgentGrant(t, r, "agent")
	keep := issueAgentGrant(t, r, "keep")
	w := doJSON(t, r, "GET", "/api/agent-grants", nil)
	require.Equal(t, 200, w.Code)
	assert.NotContains(t, w.Body.String(), issued.Token)
	assert.NotContains(t, w.Body.String(), agentscope.PersistedTokenDigest(issued.Token))
	var list struct {
		Grants []store.AgentGrant `json:"grants"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	require.Len(t, list.Grants, 2)
	plain, err := r.cfg.Store.ResolveAgentGrant(context.Background(), issued.Token)
	require.NoError(t, err)
	assert.Nil(t, plain)
	digest, err := r.cfg.Store.ResolveAgentGrant(context.Background(), agentscope.PersistedTokenDigest(issued.Token))
	require.NoError(t, err)
	require.NotNil(t, digest)
	assert.Equal(t, issued.Grant.ID, digest.ID)
	require.Equal(t, 204, doJSON(t, r, "DELETE", "/api/agent-grants/"+issued.Grant.ID, nil).Code)
	require.Equal(t, 404, doJSON(t, r, "DELETE", "/api/agent-grants/"+issued.Grant.ID, nil).Code)
	for _, path := range []string{"/api/agent/credential", "/api/artifacts"} {
		assert.Equal(t, 401, doWithToken(t, r, "GET", path, issued.Token, nil).Code)
		assert.Equal(t, 200, doWithToken(t, r, "GET", path, keep.Token, nil).Code)
	}
}

func TestAgentGrantManagementRequiresTheOwnersSession(t *testing.T) {
	in := newTwoOwnerInstance(t)
	in.ro.cfg.MCPURL = "https://agents.example.test/mcp"
	cookieOne := sessionCookieFor(t, in.ro.cfg.Store, in.ownerOne, "owner-one-grants")
	issued := decodeIssuedAgentGrant(t, doWithCookie(t, in.ro, "POST", "/api/agent-grants", cookieOne, map[string]string{"label": "Private owner one grant"}))
	ownTwo := decodeIssuedAgentGrant(t, doWithCookie(t, in.ro, "POST", "/api/agent-grants", in.cookieTwo, map[string]string{"label": "Private owner two grant"}))
	for _, tc := range []struct{ method, path string }{{"GET", "/api/agent-grants"}, {"POST", "/api/agent-grants"}, {"DELETE", "/api/agent-grants/" + issued.Grant.ID}} {
		assert.Equal(t, 403, doWithToken(t, in.ro, tc.method, tc.path, "secret", map[string]string{"label": "x"}).Code)
	}
	w := doWithCookie(t, in.ro, "GET", "/api/agent-grants", in.cookieTwo, nil)
	require.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), ownTwo.Grant.ID)
	assert.NotContains(t, w.Body.String(), issued.Grant.ID)
	w = doWithCookie(t, in.ro, "DELETE", "/api/agent-grants/"+issued.Grant.ID, in.cookieTwo, nil)
	assert.Equal(t, 404, w.Code)
	require.NoError(t, in.ro.cfg.Store.SetUserAdmin(context.Background(), in.ownerTwo, true))
	require.NoError(t, in.ro.cfg.Store.SetUserDisabled(context.Background(), in.ownerOne, true))
	assert.Equal(t, 401, doWithToken(t, in.ro, "GET", "/api/agent/credential", issued.Token, nil).Code)
}

func TestAgentGrantLabelsAndNoLoginInstance(t *testing.T) {
	r := newTestRouter(t)
	r.cfg.MCPURL = "https://agents.example.test/mcp"
	assert.Equal(t, "External agent", issueAgentGrant(t, r, " \n ").Grant.Label)
	assert.Equal(t, strings.Repeat("x", 100), issueAgentGrant(t, r, strings.Repeat("x", 101)).Grant.Label)
	assert.Equal(t, strings.Repeat("ø", 100), issueAgentGrant(t, r, strings.Repeat("ø", 100)).Grant.Label)
	cfg := r.cfg
	cfg.AuthToken = ""
	open := NewRouter(cfg)
	issued := decodeIssuedAgentGrant(t, doWithCookie(t, open, "POST", "/api/agent-grants", nil, map[string]string{"label": "Operator"}))
	assert.Equal(t, 200, doWithToken(t, open, "GET", "/api/agent/credential", issued.Token, nil).Code)
	assert.Equal(t, 401, doWithToken(t, open, "GET", "/api/artifacts", "exgrant_invalid", nil).Code)
}

func TestAgentGrantPublicVisitorCannotManageCredentials(t *testing.T) {
	r := newPublicModeRouter(t, PublicMode{Enabled: true, OwnerID: defaultOwnerID})
	r.cfg.MCPURL = "https://agents.example.test/mcp"
	issueAgentGrant(t, r, "Operator connection")
	for _, path := range []string{"/settings", "/partials/agent-grants"} {
		w := doJSON(t, r, "GET", path, nil)
		require.Equal(t, 200, w.Code)
		assert.Contains(t, w.Body.String(), "Operator connection")
	}
	for _, tc := range []struct{ method, path string }{{"GET", "/api/agent-grants"}, {"POST", "/api/agent-grants"}, {"DELETE", "/api/agent-grants/missing"}} {
		w := doWithCookie(t, r, tc.method, tc.path, nil, map[string]string{"label": "visitor"})
		assert.Equal(t, 401, w.Code, w.Body.String())
	}
	w := doWithCookie(t, r, "GET", "/partials/agent-grants", nil, nil)
	assert.NotEqual(t, 200, w.Code)
	for _, path := range []string{"/settings", "/profile"} {
		w := doWithCookie(t, r, "GET", path, nil, nil)
		assert.Equal(t, 401, w.Code)
		assert.NotContains(t, w.Body.String(), "Connected agents")
		assert.NotContains(t, w.Body.String(), "secret")
	}
}

func TestAgentGrantsPanelIsTheOwners(t *testing.T) {
	r := newTestRouter(t)
	r.cfg.MCPURL = "https://agents.example.test/mcp"
	issued := issueAgentGrant(t, r, "<b>Cursor</b>")
	w := doJSON(t, r, "GET", "/partials/agent-grants", nil)
	require.Equal(t, 200, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "&lt;b&gt;Cursor&lt;/b&gt;")
	assert.Contains(t, w.Body.String(), `data-action="revoke-agent-grant"`)
	assert.Contains(t, strings.ToLower(w.Body.String()), "never used")
	assert.NotContains(t, w.Body.String(), issued.Token)
	for _, path := range []string{"/settings", "/profile"} {
		w = doJSON(t, r, "GET", path, nil)
		require.Equal(t, 200, w.Code, w.Body.String())
		for _, id := range []string{"agent-grants-panel", "agent-grants-body", "agent-grant-form", "agent-grant-issued", "agent-grant-token", "agent-grant-copy", "agent-grant-label"} {
			assert.Contains(t, w.Body.String(), `id="`+id+`"`)
		}
		assert.Contains(t, w.Body.String(), "Connected agents")
		assert.Contains(t, w.Body.String(), "gallery/agent-grants.js")
	}
	in := newTwoOwnerInstance(t)
	in.ro.cfg.MCPURL = "https://agents.example.test/mcp"
	cookieOne := sessionCookieFor(t, in.ro.cfg.Store, in.ownerOne, "one-panel")
	decodeIssuedAgentGrant(t, doWithCookie(t, in.ro, "POST", "/api/agent-grants", cookieOne, map[string]string{"label": "OnePrivateGrant"}))
	decodeIssuedAgentGrant(t, doWithCookie(t, in.ro, "POST", "/api/agent-grants", in.cookieTwo, map[string]string{"label": "TwoPrivateGrant"}))
	for _, path := range []string{"/settings", "/partials/agent-grants"} {
		w = doWithCookie(t, in.ro, "GET", path, in.cookieTwo, nil)
		require.Equal(t, 200, w.Code, w.Body.String())
		assert.Contains(t, w.Body.String(), "TwoPrivateGrant")
		assert.NotContains(t, w.Body.String(), "OnePrivateGrant")
	}
}

func TestAgentCredentialDescribesPiArtifactScope(t *testing.T) {
	r, reg := newScopedTestRouter(t)
	id := createArtifact(t, r, map[string]any{"title": "A", "body": "<p>a</p>"})
	pi, err := reg.Issue(defaultOwnerID, id, "")
	require.NoError(t, err)
	w := doWithToken(t, r, "GET", "/api/agent/credential", pi.Token(), nil)
	require.Equal(t, 200, w.Code, w.Body.String())
	var credential map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &credential))
	assert.Equal(t, "artifact", credential["scope"])
	assert.Equal(t, id, credential["artifact_id"])
	assert.NotContains(t, credential, "grant_id")
}

func TestRevokedLibraryGrantNeverFallsBackToPublicOrNoLoginAccess(t *testing.T) {
	for _, public := range []bool{false, true} {
		t.Run(map[bool]string{false: "no login", true: "public"}[public], func(t *testing.T) {
			base := newTestRouter(t)
			base.cfg.MCPURL = "https://agents.example.test/mcp"
			cfg := base.cfg
			if !public {
				cfg.AuthToken = ""
			}
			cfg.Public = PublicMode{Enabled: public, OwnerID: defaultOwnerID}
			r := NewRouter(cfg)
			issued := issueAgentGrant(t, r, "operator")
			require.Equal(t, 204, doJSON(t, r, "DELETE", "/api/agent-grants/"+issued.Grant.ID, nil).Code)
			for _, path := range []string{"/api/artifacts", "/api/agent/credential"} {
				assert.Equal(t, 401, doWithToken(t, r, "GET", path, issued.Token, nil).Code)
			}
		})
	}
}

func TestAgentGrantsSupportLocalAndHostedAccounts(t *testing.T) {
	t.Run("local", func(t *testing.T) {
		r, st := newLoginTestRouter(t, nil, newTestCredential(t, "owner", "password"))
		r.cfg.MCPURL = "https://agents.example.test/mcp"
		user, err := st.UpsertUser(context.Background(), "local:owner", "owner@example.test")
		require.NoError(t, err)
		cookie := sessionCookieFor(t, st, user.ID, "local-grants")
		issued := decodeIssuedAgentGrant(t, doWithCookie(t, r, "POST", "/api/agent-grants", cookie, map[string]string{"label": "Local laptop"}))
		assert.Equal(t, 200, doWithToken(t, r, "GET", "/api/agent/credential", issued.Token, nil).Code)
		page := doWithCookie(t, r, "GET", "/settings", cookie, nil)
		require.Equal(t, 200, page.Code)
		assert.Contains(t, page.Body.String(), "Connected agents")
		assert.Contains(t, page.Body.String(), "Local laptop")
	})
	t.Run("Clerk", func(t *testing.T) {
		base := newTestRouter(t)
		base.cfg.MCPURL = "https://agents.example.test/mcp"
		cfg := base.cfg
		cfg.Clerk = hostedLoginStub{}
		cfg.ClerkPublishableKey = "pk_test_example"
		cfg.ClerkFrontendAPI = "https://example.clerk.accounts.dev"
		cfg.AuthToken = ""
		r := NewRouter(cfg)
		request := func(method, path, body string) *httptest.ResponseRecorder {
			req := httptest.NewRequest(method, path, strings.NewReader(body))
			req.Header.Set("Test-Clerk-Subject", "clerk:grant-owner")
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			return w
		}
		issued := decodeIssuedAgentGrant(t, request("POST", "/api/agent-grants", `{"label":"Hosted laptop"}`))
		assert.Equal(t, 200, doWithToken(t, r, "GET", "/api/agent/credential", issued.Token, nil).Code)
		for _, path := range []string{"/settings", "/partials/agent-grants"} {
			page := request("GET", path, "")
			require.Equal(t, 200, page.Code, page.Body.String())
			assert.Contains(t, page.Body.String(), "Hosted laptop")
		}
	})
}

func TestLibraryGrantScopeRejectsPathTricks(t *testing.T) {
	scope := agentscope.Persisted(defaultOwnerID, "grant", "agent").Scope()
	for _, path := range []string{"/api/artifacts/a%2Fb", "/api/artifacts/a/state/import", "/api/artifacts/a%2F..%2Fb", "/api/artifactsabc", "/api/artifacts/a/versions", "/api/artifacts/a/assets/export", "/api/agent/credential/extra"} {
		assert.False(t, agentScopeAllows(scope, "GET", path), path)
	}
	assert.True(t, agentScopeAllows(scope, "GET", "/api/artifacts/%61%62%63"))
}

// Saved data is read-only to a library grant (Exh-17a5): it is not versioned,
// so an agent's write or erase could not be undone. The refusal is the route
// policy's, before any handler, so it holds for every artifact alike.
func TestLibraryGrantCannotWriteSavedData(t *testing.T) {
	r := newTestRouter(t)
	r.cfg.MCPURL = "https://agents.example.test/mcp"
	id := createArtifact(t, r, map[string]any{"title": "Water tracker", "body": "<p>water</p>"})
	path := "/api/artifacts/" + id + "/state"
	require.Equal(t, 204, doJSON(t, r, "PUT", path, map[string]any{"key": "glasses", "value": "6"}).Code)
	issued := issueAgentGrant(t, r, "agent")

	for _, tc := range []struct {
		method, path string
		body         any
	}{
		{"PUT", path, map[string]any{"key": "glasses", "value": "0"}},
		{"DELETE", path + "?key=glasses", nil},
		{"DELETE", path, nil},
	} {
		w := doWithToken(t, r, tc.method, tc.path, issued.Token, tc.body)
		assert.Equal(t, http.StatusForbidden, w.Code, tc.method+" "+tc.path)
	}
	w := doWithToken(t, r, "GET", path, issued.Token, nil)
	require.Equal(t, 200, w.Code)
	assert.JSONEq(t, `{"glasses":"6"}`, w.Body.String())
}

func TestCoreNeverServesMCP(t *testing.T) {
	for _, endpoint := range []string{"", "https://exhibit.example.test/mcp"} {
		t.Run(endpoint, func(t *testing.T) {
			r := newTestRouter(t)
			r.cfg.MCPURL = endpoint
			for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodDelete} {
				for _, token := range []string{"", r.cfg.AuthToken} {
					w := doWithToken(t, r, method, "/mcp", token, nil)
					assert.Equal(t, http.StatusNotFound, w.Code, "%s bearer=%t: %s", method, token != "", w.Body.String())
				}
			}
		})
	}
}
