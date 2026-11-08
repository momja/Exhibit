package api

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/momja/Exhibit/internal/store"
)

// allowAgentStateByDefault turns on owner 1's account-wide permission for
// agents to use saved data (Exh-dabd), through the route Settings uses.
func allowAgentStateByDefault(t *testing.T, r *Router) {
	t.Helper()
	w := doJSON(t, r, http.MethodPut, "/api/settings/agent-state-access", map[string]any{"allowed": true})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func setArtifactAgentStateAccess(t *testing.T, r *Router, id, access string) {
	t.Helper()
	w := doJSON(t, r, http.MethodPatch, "/api/artifacts/"+id, map[string]any{"agent_state_access": access})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

// stateRoutes are every state operation an agent credential can reach.
func stateRoutes(id string) []struct {
	method, path string
	body         any
} {
	path := "/api/artifacts/" + id + "/state"
	return []struct {
		method, path string
		body         any
	}{
		{http.MethodGet, path, nil},
		{http.MethodPut, path, map[string]any{"key": "notes", "value": "changed"}},
		{http.MethodDelete, path + "?key=notes", nil},
		{http.MethodDelete, path, nil},
	}
}

// Saved data is kept from agents until the owner says otherwise, whichever
// kind of agent asks: a Pi session's artifact grant or an MCP library grant.
func TestAgentsCannotUseSavedDataByDefault(t *testing.T) {
	r, reg := newScopedTestRouter(t)
	r.cfg.MCPURL = "https://agents.example.test/mcp"
	id := createArtifact(t, r, map[string]any{"title": "Journal", "body": "<p>journal</p>"})
	require.Equal(t, http.StatusNoContent, doJSON(t, r, http.MethodPut, "/api/artifacts/"+id+"/state",
		map[string]any{"key": "notes", "value": "private"}).Code)

	pi, err := reg.Issue(1, id, "")
	require.NoError(t, err)
	library := issueAgentGrant(t, r, "External agent")

	for name, token := range map[string]string{"pi": pi.Token(), "library": library.Token} {
		for _, tc := range stateRoutes(id) {
			w := doWithToken(t, r, tc.method, tc.path, token, tc.body)
			assert.Equal(t, http.StatusForbidden, w.Code, "%s %s %s", name, tc.method, tc.path)
			assert.NotContains(t, w.Body.String(), "private", "%s %s %s", name, tc.method, tc.path)
			// A library grant never writes saved data (Exh-l2xj), so its
			// writes stop at the route policy before reaching this check.
			if name == "pi" || tc.method == http.MethodGet {
				assert.Contains(t, w.Body.String(), "owner has not allowed agents", "%s %s %s", name, tc.method, tc.path)
			}
		}
	}

	// The person's own requests are unaffected, and nothing above changed the data.
	w := doJSON(t, r, http.MethodGet, "/api/artifacts/"+id+"/state", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"notes":"private"}`, w.Body.String())
}

// The artifact's own answer wins over the account default in both directions.
func TestArtifactAgentStateAccessOverridesTheAccountDefault(t *testing.T) {
	r, reg := newScopedTestRouter(t)
	inherit := createArtifact(t, r, map[string]any{"title": "Inherit", "body": "<p>a</p>"})
	allow := createArtifact(t, r, map[string]any{"title": "Allow", "body": "<p>b</p>"})
	deny := createArtifact(t, r, map[string]any{"title": "Deny", "body": "<p>c</p>"})
	setArtifactAgentStateAccess(t, r, allow, store.AgentStateAllow)
	setArtifactAgentStateAccess(t, r, deny, store.AgentStateDeny)

	read := func(id string) int {
		g, err := reg.Issue(1, id, "")
		require.NoError(t, err)
		return doWithToken(t, r, http.MethodGet, "/api/artifacts/"+id+"/state", g.Token(), nil).Code
	}

	assert.Equal(t, http.StatusForbidden, read(inherit), "default off")
	assert.Equal(t, http.StatusOK, read(allow), "allowed for this artifact alone")
	assert.Equal(t, http.StatusForbidden, read(deny))

	allowAgentStateByDefault(t, r)
	assert.Equal(t, http.StatusOK, read(inherit), "follows the account default")
	assert.Equal(t, http.StatusOK, read(allow))
	assert.Equal(t, http.StatusForbidden, read(deny), "an explicit no outlasts a permissive default")

	setArtifactAgentStateAccess(t, r, deny, store.AgentStateInherit)
	assert.Equal(t, http.StatusOK, read(deny), "back to following the default")
}

// An agent cannot give itself the access this setting withholds.
func TestAgentsCannotChangeTheirOwnSavedDataAccess(t *testing.T) {
	r, reg := newScopedTestRouter(t)
	r.cfg.MCPURL = "https://agents.example.test/mcp"
	id := createArtifact(t, r, map[string]any{"title": "Journal", "body": "<p>journal</p>"})
	pi, err := reg.Issue(1, id, "")
	require.NoError(t, err)
	library := issueAgentGrant(t, r, "External agent")

	for name, token := range map[string]string{"pi": pi.Token(), "library": library.Token} {
		w := doWithToken(t, r, http.MethodPatch, "/api/artifacts/"+id, token,
			map[string]any{"agent_state_access": store.AgentStateAllow})
		assert.Equal(t, http.StatusForbidden, w.Code, name)
		w = doWithToken(t, r, http.MethodPut, "/api/settings/agent-state-access", token, map[string]any{"allowed": true})
		assert.Equal(t, http.StatusForbidden, w.Code, name)
		w = doWithToken(t, r, http.MethodGet, "/api/settings/agent-state-access", token, nil)
		assert.Equal(t, http.StatusForbidden, w.Code, name)
	}

	a, err := r.cfg.Store.GetArtifact(t.Context(), 1, id)
	require.NoError(t, err)
	assert.Equal(t, store.AgentStateInherit, a.AgentStateAccess)
	allowed, err := r.cfg.Store.AgentStateAccessDefault(t.Context(), 1)
	require.NoError(t, err)
	assert.False(t, allowed)
}

func TestAgentStateAccessRoutesValidateInput(t *testing.T) {
	r := newTestRouter(t)
	id := createArtifact(t, r, map[string]any{"title": "A", "body": "<p>a</p>"})

	for _, bad := range []any{"yes", true, "", nil} {
		w := doJSON(t, r, http.MethodPatch, "/api/artifacts/"+id, map[string]any{"agent_state_access": bad})
		assert.Equal(t, http.StatusBadRequest, w.Code, "%v", bad)
	}
	for _, body := range []any{map[string]any{}, map[string]any{"allowed": "true"}} {
		w := doJSON(t, r, http.MethodPut, "/api/settings/agent-state-access", body)
		assert.Equal(t, http.StatusBadRequest, w.Code, "%v", body)
	}

	w := doJSON(t, r, http.MethodGet, "/api/settings/agent-state-access", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"allowed":false}`, w.Body.String())
	allowAgentStateByDefault(t, r)
	w = doJSON(t, r, http.MethodGet, "/api/settings/agent-state-access", nil)
	assert.JSONEq(t, `{"allowed":true}`, w.Body.String())

	w = doJSON(t, r, http.MethodGet, "/api/artifacts/"+id, nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"agent_state_access":"inherit"`)
}

func TestEditPageSaysWhatTheAgentStateDefaultIs(t *testing.T) {
	a := &store.Artifact{ID: "a1", Title: "Journal", AgentStateAccess: store.AgentStateInherit}
	page, err := renderEditPage(a, nil, nil, "<p>src</p>", "", testPageCreds, testRenderURLs("https://render.test"), false, "", nil, editNavigation{}, false)
	require.NoError(t, err)
	assert.Contains(t, page, `data-controller="agent-state-access"`)
	assert.Contains(t, page, `data-agent-state-access-url-value="/api/artifacts/a1"`)
	assert.Contains(t, page, `<option value="inherit" selected>Use account default (not allowed)</option>`)

	a.AgentStateAccess = store.AgentStateDeny
	page, err = renderEditPage(a, nil, nil, "<p>src</p>", "", testPageCreds, testRenderURLs("https://render.test"), false, "", nil, editNavigation{}, true)
	require.NoError(t, err)
	assert.Contains(t, page, `<option value="inherit">Use account default (allowed)</option>`)
	assert.Contains(t, page, `<option value="deny" selected>Don't allow</option>`)
}

func TestSettingsOffersTheAgentStateDefaultOnlyWhereAgentsRun(t *testing.T) {
	r := newTestRouter(t)
	w := doJSON(t, r, http.MethodGet, "/settings", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.NotContains(t, w.Body.String(), `id="agent-state-access"`, "no agents, nothing to decide")

	r.cfg.MCPURL = "https://agents.example.test/mcp"
	w = doJSON(t, r, http.MethodGet, "/settings", nil)
	require.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, `id="agent-state-access"`)
	assert.Contains(t, body, `id="agent-state-access-default" data-agent-state-access-target="input" data-action="change->agent-state-access#save">`,
		"unchecked until the owner allows it")

	allowAgentStateByDefault(t, r)
	w = doJSON(t, r, http.MethodGet, "/settings", nil)
	assert.Contains(t, w.Body.String(), `data-action="change->agent-state-access#save" checked>`)
}
