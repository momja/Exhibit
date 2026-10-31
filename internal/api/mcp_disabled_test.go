package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentGrantsDisabledWithoutMCPURL(t *testing.T) {
	r := newTestRouter(t)
	r.cfg.MCPURL = ""
	for _, body := range []string{`{"label":"Cannot issue"}`, `{`, ``} {
		req := httptest.NewRequest(http.MethodPost, "/api/agent-grants", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+r.cfg.AuthToken)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code, "body=%q: %s", body, w.Body.String())
	}
	w := doJSON(t, r, http.MethodGet, "/api/agent-grants", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var listed listAgentGrantsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &listed))
	assert.Empty(t, listed.Grants)
	for _, path := range []string{"/settings", "/profile"} {
		w := doJSON(t, r, http.MethodGet, path, nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.NotContains(t, w.Body.String(), "Connected agents")
		assert.NotContains(t, w.Body.String(), `id="agent-grants-panel"`)
		assert.NotContains(t, w.Body.String(), `id="agent-grant-form"`)
		assert.NotContains(t, w.Body.String(), "gallery/agent-grants.js")
	}
}

func TestAgentGrantsRemainRevocableAfterMCPIsDisabled(t *testing.T) {
	r := newTestRouter(t)
	r.cfg.MCPURL = "https://agents.example.test/mcp"
	issued := issueAgentGrant(t, r, "Existing connection")
	r.cfg.MCPURL = ""

	w := doJSON(t, r, http.MethodPost, "/api/agent-grants", map[string]string{"label": "Cannot issue another"})
	require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
	var page string
	for _, path := range []string{"/settings", "/profile"} {
		w := doJSON(t, r, http.MethodGet, path, nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		page = w.Body.String()
		assert.Contains(t, page, "Connected agents")
		assert.Contains(t, page, "MCP is not enabled on this instance.")
		assert.Contains(t, page, "Existing connection")
		assert.Contains(t, page, `data-action="revoke-agent-grant"`)
		assert.NotContains(t, page, `id="agent-grant-form"`)
		assert.NotContains(t, page, `id="agent-grant-issued"`)
		assert.NotContains(t, page, `id="agent-grant-token"`)
		assert.NotContains(t, page, "Issue token")
		assert.NotContains(t, page, "data-mcp-url")
		assert.NotContains(t, page, `id="agent-mcp-url"`)
		assert.NotContains(t, page, `id="agent-mcp-command"`)
		assert.NotContains(t, page, "For clients without browser sign-in")
		assert.NotContains(t, page, issued.Token)
	}
	w = doJSON(t, r, http.MethodGet, "/api/agent-grants", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var listed listAgentGrantsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &listed))
	require.Len(t, listed.Grants, 1)
	assert.Equal(t, issued.Grant.ID, listed.Grants[0].ID)
	assert.NotContains(t, w.Body.String(), issued.Token)

	// Replay the actual htmx refresh derived from the revocation-only page.
	// Until the last revoke it must still show the owner's live connection.
	refresh := httptest.NewRecorder()
	r.ServeHTTP(refresh, agentGrantRefreshRequest(t, page))
	require.Equal(t, http.StatusOK, refresh.Code, refresh.Body.String())
	assert.Contains(t, refresh.Body.String(), issued.Grant.ID)

	w = doJSON(t, r, http.MethodDelete, "/api/agent-grants/"+issued.Grant.ID, nil)
	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
	assert.Equal(t, http.StatusUnauthorized, doWithToken(t, r, http.MethodGet, "/api/agent/credential", issued.Token, nil).Code)
	w = doJSON(t, r, http.MethodGet, "/api/agent-grants", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &listed))
	assert.Empty(t, listed.Grants)

	// A successful empty outer swap removes the section. A 404 would leave a
	// stale, already-revoked connection on the page and show a refresh error.
	refresh = httptest.NewRecorder()
	r.ServeHTTP(refresh, agentGrantRefreshRequest(t, page))
	require.Equal(t, http.StatusOK, refresh.Code, refresh.Body.String())
	assert.Equal(t, "#agent-grants-panel", refresh.Header().Get("HX-Retarget"))
	assert.Equal(t, "outerHTML", refresh.Header().Get("HX-Reswap"))
	assert.Empty(t, strings.TrimSpace(refresh.Body.String()))
	for _, path := range []string{"/settings", "/profile"} {
		w := doJSON(t, r, http.MethodGet, path, nil)
		require.Equal(t, http.StatusOK, w.Code)
		assert.NotContains(t, w.Body.String(), "Connected agents")
	}
}

func TestDisabledMCPGrantManagementRemainsOwnerScoped(t *testing.T) {
	in := newTwoOwnerInstance(t)
	in.ro.cfg.MCPURL = "https://agents.example.test/mcp"
	cookieOne := sessionCookieFor(t, in.ro.cfg.Store, in.ownerOne, "disabled-mcp-owner-one")
	foreign := decodeIssuedAgentGrant(t, doWithCookie(t, in.ro, http.MethodPost, "/api/agent-grants", cookieOne, map[string]string{"label": "ForeignPrivateConnection"}))
	in.ro.cfg.MCPURL = ""

	// Another owner's grants must not make this owner's absent panel visible.
	for _, path := range []string{"/settings", "/profile"} {
		w := doWithCookie(t, in.ro, http.MethodGet, path, in.cookieTwo, nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.NotContains(t, w.Body.String(), "Connected agents")
		assert.NotContains(t, w.Body.String(), "ForeignPrivateConnection")
	}
	w := doWithCookie(t, in.ro, http.MethodGet, "/api/agent-grants", in.cookieTwo, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), foreign.Grant.ID)
	assert.Equal(t, http.StatusNotFound, doWithCookie(t, in.ro, http.MethodDelete, "/api/agent-grants/"+foreign.Grant.ID, in.cookieTwo, nil).Code)

	// Both sessions still manage only their own existing grants when disabled.
	in.ro.cfg.MCPURL = "https://agents.example.test/mcp"
	owned := decodeIssuedAgentGrant(t, doWithCookie(t, in.ro, http.MethodPost, "/api/agent-grants", in.cookieTwo, map[string]string{"label": "OwnPrivateConnection"}))
	in.ro.cfg.MCPURL = ""
	for _, path := range []string{"/settings", "/profile", "/partials/agent-grants", "/api/agent-grants"} {
		w := doWithCookie(t, in.ro, http.MethodGet, path, in.cookieTwo, nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.Contains(t, w.Body.String(), "OwnPrivateConnection")
		assert.NotContains(t, w.Body.String(), "ForeignPrivateConnection")
		assert.NotContains(t, w.Body.String(), `id="agent-grant-form"`)
	}
	assert.Equal(t, http.StatusNotFound, doWithCookie(t, in.ro, http.MethodPost, "/api/agent-grants", in.cookieTwo, map[string]string{"label": "Cannot issue"}).Code)
	assert.Equal(t, http.StatusNoContent, doWithCookie(t, in.ro, http.MethodDelete, "/api/agent-grants/"+owned.Grant.ID, in.cookieTwo, nil).Code)
	w = doWithCookie(t, in.ro, http.MethodGet, "/api/agent-grants", cookieOne, nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), foreign.Grant.ID)
	assert.Equal(t, http.StatusNoContent, doWithCookie(t, in.ro, http.MethodDelete, "/api/agent-grants/"+foreign.Grant.ID, cookieOne, nil).Code)
}

func TestEnabledMCPPanelOffersRemoteConnectionConfiguration(t *testing.T) {
	r := newTestRouter(t)
	r.cfg.MCPURL = "https://agents.example.test/connector/mcp"
	for _, path := range []string{"/settings", "/profile"} {
		w := doJSON(t, r, http.MethodGet, path, nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		page := w.Body.String()
		assert.Contains(t, page, `data-mcp-url="`+r.cfg.MCPURL+`"`)
		for _, id := range []string{"agent-grant-form", "agent-grant-token", "agent-grant-copy", "agent-grant-remote-command", "agent-grant-remote-config"} {
			assert.Contains(t, page, `id="`+id+`"`)
		}
		assert.Contains(t, page, "Claude Code")
		assert.NotContains(t, page, "MCP is not enabled")
		assert.NotContains(t, page, "go install")
		assert.NotContains(t, page, `id="agent-grant-command"`)
		assert.NotContains(t, page, "data-app-origin")
	}
}
