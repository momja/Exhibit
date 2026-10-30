package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The remote endpoint (Exh-uhvu): a remote MCP client holding a pasted grant
// token edits its artifact, and nothing but such a token is accepted.

type headerTransport struct {
	header http.Header
	base   http.RoundTripper
}

func (h headerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	for k, v := range h.header {
		r.Header[k] = v
	}
	return h.base.RoundTrip(r)
}

func connectRemoteMCP(t *testing.T, url string, header http.Header) (*mcp.ClientSession, error) {
	t.Helper()
	transport := &mcp.StreamableClientTransport{Endpoint: url,
		HTTPClient: &http.Client{Transport: headerTransport{header: header, base: http.DefaultTransport}}}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "remote-test", Version: "test"}, nil).
		Connect(context.Background(), transport, nil)
	if err == nil {
		t.Cleanup(func() { _ = cs.Close() })
	}
	return cs, err
}

func bearer(token string) http.Header {
	return http.Header{"Authorization": {"Bearer " + token}}
}

func TestRemoteMCPClientEditsItsArtifact(t *testing.T) {
	r := newTestRouter(t)
	id := createArtifact(t, r, map[string]any{"title": "A", "body": "<html><body><p>0</p></body></html>"})
	tok, _ := issueAgentGrant(t, r, id, "Claude Desktop")
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	cs, err := connectRemoteMCP(t, srv.URL+"/mcp", bearer(tok))
	require.NoError(t, err)
	h := mcpHarness{r: r, id: id, session: cs}

	out, isErr := h.call(t, "get_artifact", nil)
	require.False(t, isErr, out)
	assert.Contains(t, out, "<p>0</p>")
	out, isErr = h.call(t, "edit_artifact", map[string]any{"edits": []map[string]string{{"oldText": "<p>0</p>", "newText": "<p>remote</p>"}}})
	require.False(t, isErr, out)
	assert.Contains(t, h.body(t), "<p>remote</p>")

	w := doJSON(t, r, "GET", "/api/artifacts/"+id+"/versions", nil)
	assert.Contains(t, w.Body.String(), `"message":"Claude Desktop"`)
	assert.Contains(t, w.Body.String(), `"origin":"agent"`)
}

// postInitialize sends one raw initialize request, which is all a refused
// credential ever gets to send.
func postInitialize(t *testing.T, r http.Handler, header http.Header, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", "/mcp", strings.NewReader(
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"x","version":"0"}}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	for k, v := range header {
		req.Header[k] = v
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestRemoteMCPAcceptsOnlyAnAgentGrant(t *testing.T) {
	in := newTwoOwnerInstance(t)
	cookie := sessionCookieFor(t, in.ro.cfg.Store, in.ownerOne, "session-owner-one")
	w := doWithCookie(t, in.ro, "POST", "/api/artifacts/"+in.artifactOne+"/agent-grants", cookie, map[string]any{"label": "x"})
	require.Equal(t, http.StatusCreated, w.Code)
	tok := decodeToken(t, w)

	assert.Equal(t, http.StatusUnauthorized, postInitialize(t, in.ro, nil, nil).Code, "no credential")
	assert.Equal(t, http.StatusUnauthorized, postInitialize(t, in.ro, nil, cookie).Code, "a session cookie is never read here")
	assert.Equal(t, http.StatusUnauthorized, postInitialize(t, in.ro, bearer("secret"), nil).Code, "nor is the service token")
	assert.Equal(t, http.StatusUnauthorized, postInitialize(t, in.ro, bearer("exgrant_forged"), nil).Code)
	assert.Equal(t, http.StatusOK, postInitialize(t, in.ro, bearer(tok), nil).Code)

	// A page on another origin is refused even with a valid token.
	foreign := bearer(tok)
	foreign.Set("Origin", "https://evil.example")
	assert.Equal(t, http.StatusForbidden, postInitialize(t, in.ro, foreign, nil).Code)
	own := bearer(tok)
	own.Set("Origin", in.ro.cfg.AppOrigin)
	assert.Equal(t, http.StatusOK, postInitialize(t, in.ro, own, nil).Code)
}

func TestRemoteMCPPiSessionTokenIsRefused(t *testing.T) {
	r, reg := newScopedTestRouter(t)
	id := createArtifact(t, r, map[string]any{"title": "A", "body": "<html><body>a</body></html>"})
	g, err := reg.Issue(1, id, "")
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, postInitialize(t, r, bearer(g.Token()), nil).Code)
}

func TestRemoteMCPRevokedGrantStopsTheSession(t *testing.T) {
	r := newTestRouter(t)
	id := createArtifact(t, r, map[string]any{"title": "A", "body": "<html><body>a</body></html>"})
	tok, grant := issueAgentGrant(t, r, id, "agent")
	other, _ := issueAgentGrant(t, r, id, "other")
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	cs, err := connectRemoteMCP(t, srv.URL+"/mcp", bearer(tok))
	require.NoError(t, err)
	h := mcpHarness{r: r, id: id, session: cs}
	_, isErr := h.call(t, "get_state", nil)
	require.False(t, isErr)

	// A different grant cannot ride an open session, even over the same artifact.
	req, err := http.NewRequest("POST", srv.URL+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":9,"method":"tools/list"}`))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+other)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Mcp-Session-Id", cs.ID())
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	assert.NotEqual(t, http.StatusOK, resp.StatusCode)

	w := doJSON(t, r, "DELETE", "/api/artifacts/"+id+"/agent-grants/"+grant.ID, nil)
	require.Equal(t, http.StatusNoContent, w.Code)
	_, err = cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_state", Arguments: map[string]any{}})
	assert.Error(t, err, "the next request is a 401")
}

func decodeToken(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	body := w.Body.String()
	i := strings.Index(body, `"token":"`)
	require.GreaterOrEqual(t, i, 0, body)
	rest := body[i+len(`"token":"`):]
	return rest[:strings.Index(rest, `"`)]
}
