package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/momja/Exhibit/internal/agentscope"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These cover the enforced half of av-e0yj. The agent's tools take no artifact
// id, which is what stops a model from being talked into naming one — but the
// guarantee must not rest on the extension staying honest. Every request the
// sidecar makes carries a per-session credential the API resolves to a scope,
// and everything outside that scope is refused here, before any handler runs.

// newScopedTestRouter returns a router with an agent credential registry
// attached, plus the registry so a test can issue grants the way a spawned
// session would.
func newScopedTestRouter(t *testing.T) (*Router, *agentscope.Registry) {
	t.Helper()
	r := newTestRouter(t)
	reg := agentscope.NewRegistry()
	r.cfg.AgentCredentials = reg
	return r, reg
}

func doWithToken(t *testing.T, r *Router, method, path, token string, body any) *httptest.ResponseRecorder {
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
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// Acceptance criterion 1: a session bound to A cannot write to B, and the
// refusal is the server's. This is the request a prompt-injected model would
// produce if it could name a target at all.
func TestAgentCredentialCannotWriteAnotherArtifact(t *testing.T) {
	r, reg := newScopedTestRouter(t)

	idA := createArtifact(t, r, map[string]any{"title": "A", "body": "<html><body>a</body></html>"})
	idB := createArtifact(t, r, map[string]any{"title": "B", "body": "<html><body>b</body></html>"})

	grant, err := reg.Issue(1, idA)
	require.NoError(t, err)
	tok := grant.Token()

	// Its own artifact: allowed.
	w := doWithToken(t, r, "PATCH", "/api/artifacts/"+idA, tok, map[string]any{"body": "<html><body>a2</body></html>"})
	require.Equal(t, http.StatusOK, w.Code)

	// Somebody else's: refused, and B is untouched.
	w = doWithToken(t, r, "PATCH", "/api/artifacts/"+idB, tok, map[string]any{"body": "<html><body>OWNED</body></html>"})
	assert.Equal(t, http.StatusForbidden, w.Code)

	w = doJSON(t, r, "GET", "/api/artifacts/"+idB+"?body=true", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var stored struct {
		Body string `json:"body"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &stored))
	assert.Equal(t, "<html><body>b</body></html>", stored.Body)
}

// Acceptance criterion 2: reads are scoped the same way. Reading another
// artifact is how a hostile body would get its neighbours into the model's
// context — and from there to the configured provider.
func TestAgentCredentialCannotReadAnotherArtifact(t *testing.T) {
	r, reg := newScopedTestRouter(t)

	idA := createArtifact(t, r, map[string]any{"title": "A", "body": "<html><body>a</body></html>"})
	idB := createArtifact(t, r, map[string]any{"title": "B", "body": "<html><body>secret</body></html>"})

	grant, err := reg.Issue(1, idA)
	require.NoError(t, err)

	w := doWithToken(t, r, "GET", "/api/artifacts/"+idA+"?body=true", grant.Token(), nil)
	require.Equal(t, http.StatusOK, w.Code)

	w = doWithToken(t, r, "GET", "/api/artifacts/"+idB+"?body=true", grant.Token(), nil)
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.NotContains(t, w.Body.String(), "secret")
}

// The credential is not a general API token. Everything a session has no
// business touching — the user's provider key above all — is refused by the
// same deny-by-default check, so adding a route never silently widens it.
func TestAgentCredentialReachesNothingButItsArtifact(t *testing.T) {
	r, reg := newScopedTestRouter(t)

	id := createArtifact(t, r, map[string]any{"title": "A", "body": "<html><body>a</body></html>"})
	other := createArtifact(t, r, map[string]any{"title": "B", "body": "<html><body>b</body></html>"})
	grant, err := reg.Issue(1, id)
	require.NoError(t, err)
	tok := grant.Token()

	refused := []struct{ method, path string }{
		{"GET", "/api/agent/key"},                             // the BYO provider key
		{"PUT", "/api/agent/key"},                             //
		{"DELETE", "/api/artifacts/" + id},                    // deleting its own artifact
		{"POST", "/api/artifacts"},                            // a second artifact (already bound)
		{"GET", "/api/artifacts"},                             // enumerating the library
		{"POST", "/api/shares"},                               // minting a public link
		{"GET", "/api/tags"},                                  //
		{"POST", "/api/artifacts/" + id + "/refetch"},         // re-fetching from source
		{"GET", "/api/artifacts/" + id + "/transcripts"},      // other sessions' conversations
		{"POST", "/api/artifacts/" + id + "/tags/1"},          // library organisation
		{"POST", "/api/artifacts/" + id + "/widget/generate"}, // an agent spawning an agent
		// The sub-resources this session's own tools use are its business on
		// its OWN artifact only — never on a neighbour's.
		{"GET", "/api/artifacts/" + other + "/state"},
		{"PUT", "/api/artifacts/" + other + "/state"},
		{"DELETE", "/api/artifacts/" + other + "/state"},
		{"GET", "/api/artifacts/" + other + "/widget"},
		{"PUT", "/api/artifacts/" + other + "/widget"},
	}

	for _, c := range refused {
		w := doWithToken(t, r, c.method, c.path, tok, map[string]any{})
		assert.Equalf(t, http.StatusForbidden, w.Code, "%s %s should be refused", c.method, c.path)
	}
}

// The counterpart: the state and widget routes the agent's own tools call
// (av-lvi1, av-fafu) do reach the session's own artifact. Those tools shipped
// after av-e0yj was written, and a scope that refused them would break them
// silently — the extension would 403 at runtime with no test noticing. They
// are allowlisted by name in agentSubResources, one entry per shipped tool.
func TestAgentCredentialReachesItsOwnArtifactSubResources(t *testing.T) {
	r, reg := newScopedTestRouter(t)

	id := createArtifact(t, r, map[string]any{"title": "A", "body": "<html><body>a</body></html>"})
	grant, err := reg.Issue(1, id)
	require.NoError(t, err)
	tok := grant.Token()

	w := doWithToken(t, r, "PUT", "/api/artifacts/"+id+"/state", tok,
		map[string]any{"key": "count", "value": "7"})
	require.Equal(t, http.StatusNoContent, w.Code)

	w = doWithToken(t, r, "GET", "/api/artifacts/"+id+"/state", tok, nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "count")

	w = doWithToken(t, r, "PUT", "/api/artifacts/"+id+"/widget", tok,
		map[string]any{"body": "<html><body>tile</body></html>"})
	require.Equal(t, http.StatusOK, w.Code)

	w = doWithToken(t, r, "GET", "/api/artifacts/"+id+"/widget", tok, nil)
	assert.Equal(t, http.StatusOK, w.Code)
}

// A create-mode session may create exactly one artifact. The create binds the
// credential — from the row the handler wrote, not from anything the model
// said — and the second create is refused, so a session cannot walk itself
// across the library one new artifact at a time.
func TestAgentCredentialBindsOnFirstCreate(t *testing.T) {
	r, reg := newScopedTestRouter(t)

	grant, err := reg.Issue(1, "")
	require.NoError(t, err)
	tok := grant.Token()
	require.Empty(t, grant.Scope().ArtifactID)

	w := doWithToken(t, r, "POST", "/api/artifacts", tok, map[string]any{
		"title": "Made by the agent", "body": "<html><body>hi</body></html>",
	})
	require.Equal(t, http.StatusCreated, w.Code)
	var created struct {
		Artifact struct {
			ID string `json:"id"`
		} `json:"artifact"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	require.NotEmpty(t, created.Artifact.ID)
	assert.Equal(t, created.Artifact.ID, grant.Scope().ArtifactID)

	// Bound now: it can edit what it made...
	w = doWithToken(t, r, "PATCH", "/api/artifacts/"+created.Artifact.ID, tok,
		map[string]any{"body": "<html><body>v2</body></html>"})
	assert.Equal(t, http.StatusOK, w.Code)

	// ...and cannot create another.
	w = doWithToken(t, r, "POST", "/api/artifacts", tok, map[string]any{
		"title": "And another", "body": "<html><body>hi</body></html>",
	})
	assert.Equal(t, http.StatusForbidden, w.Code)
}

// An unbound session cannot reach into the library while it waits to bind.
func TestUnboundAgentCredentialCannotTouchExistingArtifacts(t *testing.T) {
	r, reg := newScopedTestRouter(t)

	id := createArtifact(t, r, map[string]any{"title": "Existing", "body": "<html><body>x</body></html>"})
	grant, err := reg.Issue(1, "")
	require.NoError(t, err)

	w := doWithToken(t, r, "GET", "/api/artifacts/"+id, grant.Token(), nil)
	assert.Equal(t, http.StatusForbidden, w.Code)
	w = doWithToken(t, r, "PATCH", "/api/artifacts/"+id, grant.Token(), map[string]any{"body": "<html>x</html>"})
	assert.Equal(t, http.StatusForbidden, w.Code)
}

// A revoked credential — the session ended — authenticates as nothing at all.
func TestRevokedAgentCredentialIsUnauthorized(t *testing.T) {
	r, reg := newScopedTestRouter(t)

	id := createArtifact(t, r, map[string]any{"title": "A", "body": "<html><body>a</body></html>"})
	grant, err := reg.Issue(1, id)
	require.NoError(t, err)

	w := doWithToken(t, r, "GET", "/api/artifacts/"+id, grant.Token(), nil)
	require.Equal(t, http.StatusOK, w.Code)

	reg.Revoke(grant)
	w = doWithToken(t, r, "GET", "/api/artifacts/"+id, grant.Token(), nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// The scope is matched against the decoded path segment, so an id dressed up
// with percent-escapes or path traversal cannot masquerade as the session's
// own artifact.
func TestAgentScopeAllowsRejectsPathTricks(t *testing.T) {
	scope := agentscope.Scope{OwnerID: 1, ArtifactID: "abc"}

	allowed := []struct{ method, path string }{
		{"GET", "/api/artifacts/abc"},
		{"PATCH", "/api/artifacts/abc"},
	}
	for _, c := range allowed {
		assert.Truef(t, agentScopeAllows(scope, c.method, c.path), "%s %s", c.method, c.path)
	}

	allowed = []struct{ method, path string }{
		{"GET", "/api/artifacts/abc/state"},
		{"PUT", "/api/artifacts/abc/state"},
		{"DELETE", "/api/artifacts/abc/state"},
		{"GET", "/api/artifacts/abc/widget"},
		{"PUT", "/api/artifacts/abc/widget"},
		{"DELETE", "/api/artifacts/abc/widget"},
	}
	for _, c := range allowed {
		assert.Truef(t, agentScopeAllows(scope, c.method, c.path), "%s %s", c.method, c.path)
	}

	refused := []struct{ method, path string }{
		{"GET", "/api/artifacts/abd"},
		{"GET", "/api/artifacts/abd/state"},
		{"POST", "/api/artifacts/abc/state"},
		{"GET", "/api/artifacts/abc/transcripts"},
		{"POST", "/api/artifacts/abc/widget/generate"},
		{"POST", "/api/artifacts/abc/tags/1"},
		{"GET", "/api/artifacts/abc%2F..%2Fxyz"},
		{"GET", "/api/artifacts/%61%62%64"}, // "abd", escaped
		{"DELETE", "/api/artifacts/abc"},
		{"POST", "/api/artifacts"}, // bound session: no more creates
		{"GET", "/api/artifactsabc"},
		{"GET", "/api/agent/key"},
		{"GET", "/"},
	}
	for _, c := range refused {
		assert.Falsef(t, agentScopeAllows(scope, c.method, c.path), "%s %s", c.method, c.path)
	}

	// Escaped or not, the session's own id resolves to the same artifact.
	assert.True(t, agentScopeAllows(scope, "GET", "/api/artifacts/%61%62%63"))
}
