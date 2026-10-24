package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/momja/Exhibit/internal/chromium"
	"github.com/momja/Exhibit/internal/chromium/chromiumtest"
	"github.com/momja/Exhibit/internal/connectors"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An already-approved stylesheet must load alongside multiple connections in
// both host pages, without a prompt or any saved-data cleanup. A new origin
// still requires ordinary approval while those connections remain attached.
func TestNetworkOriginBrowserKeepsApprovedStylesheetAlongsideConnections(t *testing.T) {
	browser, err := chromium.New(chromiumtest.Start(t))
	require.NoError(t, err)
	ctx, cancel, err := browser.Tab(context.Background())
	require.NoError(t, err)
	defer cancel()
	ctx, timeout := context.WithTimeout(ctx, 40*time.Second)
	defer timeout()

	var stylesheetCalls atomic.Int64
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stylesheetCalls.Add(1)
		w.Header().Set("Content-Type", "text/css")
		fmt.Fprint(w, "h1 { color: rgb(12, 34, 56); }")
	}))
	defer cdn.Close()
	ro := newTestRouter(t)
	ro.cfg.Relay.Enabled = true
	ro.cfg.Connections = connectors.New(connectors.Config{Store: ro.cfg.Store.(*store.SQLiteStore), Box: ro.cfg.Secrets})
	ro.cfg.ConnectionPolicy = ro.cfg.Connections
	apiOrigins := []string{"https://first.example.test", "https://second.example.test"}
	body := `<!doctype html><html><head><link rel="stylesheet" href="` + cdn.URL + `/theme.css"></head><body><h1>Connected app</h1></body></html>`
	id := createArtifact(t, ro, map[string]any{
		"title": "Connected app", "body": body,
		"network_allowlist": append([]string{cdn.URL}, apiOrigins...),
	})
	connections := []*connectors.Connection{}
	for _, apiOrigin := range apiOrigins {
		connection, err := ro.cfg.Connections.Create(t.Context(), 1, connectors.CreateRequest{
			Name: "Example API", Recipe: "bearer", Origin: apiOrigin, Secret: "fixture-only-credential",
		}, connectors.UnlimitedLimits())
		require.NoError(t, err)
		_, err = ro.cfg.Connections.Grant(t.Context(), 1, id, connectors.GrantRequest{ConnectionID: connection.ID})
		require.NoError(t, err)
		connections = append(connections, connection)
	}
	before, err := ro.cfg.Store.ListOriginDecisions(t.Context(), 1, id)
	require.NoError(t, err)

	appMux, renderMux := http.NewServeMux(), http.NewServeMux()
	app := httptest.NewServer(appMux)
	defer app.Close()
	frame := httptest.NewServer(renderMux)
	defer frame.Close()
	ro.cfg.AppOrigin, ro.cfg.RenderOrigin = app.URL, frame.URL
	renderMux.Handle("/", ro.RenderHandler())
	appMux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Header.Set("Authorization", authHeader())
		ro.ServeHTTP(w, r)
	}))
	run := func(actions ...chromedp.Action[chromedp.Void]) {
		t.Helper()
		require.NoError(t, chromedp.Do(ctx, actions...))
	}
	check := func(expression string) {
		t.Helper()
		ok, err := chromedp.Run(ctx, chromedp.Evaluate[bool](expression))
		require.NoError(t, err)
		require.True(t, ok, expression)
	}
	for _, path := range []string{"/artifacts/" + id, "/agent?artifact=" + id} {
		callsBefore := stylesheetCalls.Load()
		run(chromedp.Navigate(app.URL + path))
		require.Eventually(t, func() bool { return stylesheetCalls.Load() > callsBefore }, 5*time.Second, 10*time.Millisecond)
		if path == "/agent?artifact="+id {
			// This fixture has no model API key; dismiss the agent's independent
			// first-use dialog before interacting with the preview's notice.
			run(chromedp.Poll[chromedp.Void](`!document.getElementById('key-modal').hidden`),
				chromedp.Click(chromedp.CSS(`#key-modal button[onclick="closeKeyModal()"]`)))
		}
		check(`document.getElementById('net-modal').hidden`)
	}
	after, err := ro.cfg.Store.ListOriginDecisions(t.Context(), 1, id)
	require.NoError(t, err)
	assert.Equal(t, before, after, "both pages use the saved approval without rewriting it")

	// Attaching connections cannot veto an ordinary new-origin approval.
	ordinaryID := createArtifact(t, ro, map[string]any{
		"title": "Ordinary app", "body": body, "network_allowlist": []string{},
	})
	for _, connection := range connections {
		_, err = ro.cfg.Connections.Grant(t.Context(), 1, ordinaryID, connectors.GrantRequest{ConnectionID: connection.ID})
		require.NoError(t, err)
	}
	callsBefore := stylesheetCalls.Load()
	run(chromedp.Navigate(app.URL+"/artifacts/"+ordinaryID), chromedp.Poll[chromedp.Void](`
!document.getElementById('net-modal')?.hidden`))
	check(`document.getElementById('net-title').innerText === 'Allow network access?'`)
	check(`getComputedStyle(document.getElementById('net-allow')).display !== 'none' && getComputedStyle(document.getElementById('net-never')).display !== 'none'`)
	check(`document.getElementById('net-once').innerText === 'Block once'`)
	assert.Equal(t, callsBefore, stylesheetCalls.Load(), "the unapproved request is blocked before approval")
	// Approval renews the trusted host as well as its frame. Wait for that
	// navigation before evaluating the new page's modal state.
	approved, err := chromedp.RunResponse(ctx, chromedp.Click(chromedp.CSS("#net-allow")))
	require.NoError(t, err)
	require.NotNil(t, approved)
	assert.EqualValues(t, http.StatusOK, approved.Status)
	check(`document.getElementById('net-modal').hidden`)
	require.Eventually(t, func() bool { return stylesheetCalls.Load() > callsBefore }, 5*time.Second, 10*time.Millisecond)
	allowed, err := ro.cfg.Store.AllowedOrigins(t.Context(), 1, ordinaryID)
	require.NoError(t, err)
	assert.Contains(t, allowed, cdn.URL)
}
