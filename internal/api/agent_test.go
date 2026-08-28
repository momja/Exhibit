package api

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/momja/Exhibit/internal/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The key round trip: PUT stores it encrypted, GET returns only a masked
// hint, DELETE unsets. The plaintext key must never come back and must not
// be stored in the clear.
func TestAgentKeyLifecycle(t *testing.T) {
	r := newTestRouter(t)

	w := doJSON(t, r, "GET", "/api/agent/key", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"configured":false`)

	w = doJSON(t, r, "PUT", "/api/agent/key", map[string]string{
		"provider": "anthropic",
		"model":    "claude-sonnet-4-5",
		"api_key":  "sk-ant-supersecret-1234",
	})
	require.Equal(t, http.StatusOK, w.Code)
	assert.NotContains(t, w.Body.String(), "supersecret")

	w = doJSON(t, r, "GET", "/api/agent/key", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var got struct {
		Configured bool   `json:"configured"`
		Provider   string `json:"provider"`
		Model      string `json:"model"`
		KeyHint    string `json:"key_hint"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.True(t, got.Configured)
	assert.Equal(t, "anthropic", got.Provider)
	assert.Equal(t, "claude-sonnet-4-5", got.Model)
	assert.Equal(t, "sk-…1234", got.KeyHint)
	assert.NotContains(t, w.Body.String(), "supersecret")

	// The stored ciphertext must not contain the plaintext key.
	k, err := r.cfg.Store.GetAgentKey(t.Context(), 1)
	require.NoError(t, err)
	require.NotNil(t, k)
	assert.NotContains(t, k.KeyCiphertext, "supersecret")

	w = doJSON(t, r, "DELETE", "/api/agent/key", nil)
	require.Equal(t, http.StatusNoContent, w.Code)

	w = doJSON(t, r, "GET", "/api/agent/key", nil)
	assert.Contains(t, w.Body.String(), `"configured":false`)
}

// The agent page's header back link must mirror the two ways it's reached
// rather than always pointing at the gallery: the add-artifact page's "Build
// with agent" tile links to bare /agent, so back must return to /new; the
// detail page's "Modify with agent" link carries ?artifact=<id>, so back
// must return to that artifact's detail page. A hardcoded "/" back link
// strands a visitor who went add-artifact -> agent -> back on the gallery
// instead of where they came from.
func TestAgentPageBackLinkMirrorsEntryPoint(t *testing.T) {
	r := newTestRouter(t)

	page := getPage(t, r, "/agent")
	assert.Contains(t, page, `<a href="/new">←<span class="lbl"> Back</span></a>`)

	id := createArtifact(t, r, map[string]any{
		"title":             "Modify me",
		"body":              "<html><body>hi</body></html>",
		"network_allowlist": []string{},
	})
	page = getPage(t, r, "/agent?artifact="+id)
	assert.Contains(t, page, `<a href="/artifacts/`+id+`">←<span class="lbl"> Back</span></a>`)
}

// Saving with an empty api_key should keep the existing key while still
// updating the model — the fix for Exh-454g, where changing only the model
// used to require re-entering the API key.
func TestAgentKeyUpdateModelKeepsExistingKey(t *testing.T) {
	r := newTestRouter(t)

	w := doJSON(t, r, "PUT", "/api/agent/key", map[string]string{
		"provider": "anthropic",
		"model":    "claude-sonnet-4-5",
		"api_key":  "sk-ant-supersecret-1234",
	})
	require.Equal(t, http.StatusOK, w.Code)
	before, err := r.cfg.Store.GetAgentKey(t.Context(), 1)
	require.NoError(t, err)

	w = doJSON(t, r, "PUT", "/api/agent/key", map[string]string{
		"provider": "anthropic",
		"model":    "claude-opus-4-8",
		"api_key":  "",
	})
	require.Equal(t, http.StatusOK, w.Code)

	var got struct {
		Provider string `json:"provider"`
		Model    string `json:"model"`
		KeyHint  string `json:"key_hint"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, "anthropic", got.Provider)
	assert.Equal(t, "claude-opus-4-8", got.Model)
	assert.Equal(t, "sk-…1234", got.KeyHint)

	after, err := r.cfg.Store.GetAgentKey(t.Context(), 1)
	require.NoError(t, err)
	assert.Equal(t, before.KeyCiphertext, after.KeyCiphertext)
	assert.Equal(t, "claude-opus-4-8", after.Model)
}

// An empty api_key with no key configured yet, or with a different provider
// selected, must still be rejected — there is no existing key to fall back to.
func TestAgentKeyRequiresKeyWhenNoneToReuse(t *testing.T) {
	r := newTestRouter(t)

	w := doJSON(t, r, "PUT", "/api/agent/key", map[string]string{
		"provider": "anthropic",
		"model":    "claude-sonnet-4-5",
		"api_key":  "",
	})
	assert.Equal(t, http.StatusBadRequest, w.Code)

	w = doJSON(t, r, "PUT", "/api/agent/key", map[string]string{
		"provider": "anthropic",
		"model":    "claude-sonnet-4-5",
		"api_key":  "sk-ant-supersecret-1234",
	})
	require.Equal(t, http.StatusOK, w.Code)

	w = doJSON(t, r, "PUT", "/api/agent/key", map[string]string{
		"provider": "openai",
		"model":    "gpt-5.2",
		"api_key":  "",
	})
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAgentKeyRejectsUnknownProvider(t *testing.T) {
	r := newTestRouter(t)
	w := doJSON(t, r, "PUT", "/api/agent/key", map[string]string{
		"provider": "definitely-not-a-provider",
		"api_key":  "k",
	})
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// Without an agent manager configured (no pi binary), session routes degrade
// to 503 rather than panicking.
func TestAgentSessionsUnavailableWithoutManager(t *testing.T) {
	r := newTestRouter(t)
	w := doJSON(t, r, "POST", "/api/agent/sessions", map[string]string{})
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

// The SSE route's only credential is a session-bound ticket. The service
// token — in the query string or anywhere else — must not open it (av-rgp1),
// and neither must a ticket minted for another session.
func TestAgentEventsAcceptsOnlyASessionTicket(t *testing.T) {
	r := newTestRouter(t)

	get := func(url string) int {
		req := httptest.NewRequest("GET", url, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w.Code
	}

	assert.Equal(t, http.StatusUnauthorized, get("/api/agent/sessions/nope/events"))
	// The old contract is gone: the master token is not a stream credential.
	assert.Equal(t, http.StatusUnauthorized, get("/api/agent/sessions/nope/events?token=secret"))
	assert.Equal(t, http.StatusUnauthorized, get("/api/agent/sessions/nope/events?ticket=made-up"))

	tkt, err := r.sseTickets.Issue("nope", 1)
	require.NoError(t, err)
	// A ticket for a different session is no better than an invented one.
	assert.Equal(t, http.StatusUnauthorized, get("/api/agent/sessions/other/events?ticket="+tkt))
	// The right ticket authenticates; with no manager the route then 503s.
	assert.Equal(t, http.StatusServiceUnavailable, get("/api/agent/sessions/nope/events?ticket="+tkt))
	// ...and it is spent, so a replay from a log line is worthless.
	assert.Equal(t, http.StatusUnauthorized, get("/api/agent/sessions/nope/events?ticket="+tkt))
}

// AC1: nothing any page requests carries the service token in a URL. The SSE
// stream — the only request that ever did — is built in exactly one place
// (api.js's apiEventSource), and what it puts in the query string is a ticket.
func TestClientNeverPutsTokenInAURL(t *testing.T) {
	entries, err := embeddedAssets.ReadDir("assets/gallery")
	require.NoError(t, err)
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".js") {
			continue
		}
		name := "assets/gallery/" + e.Name()
		js, err := embeddedAssets.ReadFile(name)
		require.NoError(t, err, name)
		assert.NotContains(t, string(js), "token=",
			name+": no request may put the token in a query string")
	}

	api, err := embeddedAssets.ReadFile("assets/gallery/api.js")
	require.NoError(t, err)
	assert.Contains(t, string(api), "'ticket=' + encodeURIComponent(ticket",
		"the SSE stream authenticates with a ticket")
}

// AC2: with debug logging on — the setting an operator reaches for when the
// agent surface misbehaves — the request log must not contain the service
// token. The request log records the raw query at debug level, so this is the
// regression the ticket scheme exists to prevent.
func TestDebugRequestLogNeverContainsTheServiceToken(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })
	require.True(t, logging.DebugEnabled())

	r := newTestRouter(t)
	tkt, err := r.sseTickets.Issue("sess-1", 1)
	require.NoError(t, err)

	// Every request the chat page makes, in order: session create, ticket
	// mint, the stream itself. This router has no Agent configured, so both
	// resolve 503 rather than actually minting anything — that's fine, the
	// query-string check below cares whether *these calls* ever wrote the
	// token into the log, not whether they succeeded.
	assert.Equal(t, http.StatusServiceUnavailable, doJSON(t, r, "POST", "/api/agent/sessions", map[string]string{}).Code)
	assert.Equal(t, http.StatusServiceUnavailable, doJSON(t, r, "POST", "/api/agent/sessions/sess-1/ticket", nil).Code)
	req := httptest.NewRequest("GET", "/api/agent/sessions/sess-1/events?ticket="+tkt, nil)
	r.ServeHTTP(httptest.NewRecorder(), req)

	logged := buf.String()
	require.Contains(t, logged, "ticket=", "the debug log does record the raw query")
	assert.NotContains(t, logged, "secret", "the service token must not reach the log")
}

// AC5 — a session belonging to another owner is not this request's to use — is
// covered by agent_session_owner_test.go, which walks every session route with
// two real accounts and a live pi subprocess (av-ep8k). The stream's half of it
// is that redeeming a ticket resolves the owner the ticket was minted for, so
// agentSession applies the same check here as everywhere else.

func TestAgentPageServes(t *testing.T) {
	r := newTestRouter(t)
	req := httptest.NewRequest("GET", "/agent", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.True(t, strings.Contains(w.Body.String(), "Agent API key"))
}
