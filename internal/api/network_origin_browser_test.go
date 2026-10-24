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

// Exercise the real render CSP, runtime reporter, trusted prompt, htmx review
// and CSS together. The approval was already persisted: connection custody is
// the reason this stylesheet remains blocked, including in the agent preview.
func TestNetworkOriginBrowserExplainsConnectionPolicyAndPreservesApproval(t *testing.T) {
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
	body := `<!doctype html><html><head><link rel="stylesheet" href="` + cdn.URL + `/theme.css"></head><body><h1>Connected app</h1></body></html>`
	id := createArtifact(t, ro, map[string]any{
		"title": "Connected app", "body": body,
		"network_allowlist": []string{cdn.URL, originPolicyAPI},
	})
	connection, err := ro.cfg.Connections.Create(t.Context(), 1, connectors.CreateRequest{
		Name: "Example API", Recipe: "bearer", Origin: originPolicyAPI, Secret: "fixture-only-credential",
	}, connectors.UnlimitedLimits())
	require.NoError(t, err)
	_, err = ro.cfg.Connections.Grant(t.Context(), 1, id, connectors.GrantRequest{ConnectionID: connection.ID})
	require.NoError(t, err)
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
		run(chromedp.Navigate(app.URL+path), chromedp.Poll[chromedp.Void](`
!document.getElementById('net-modal')?.hidden && !!document.querySelector('#net-warning-list [data-connection-data-blocked]')`))
		if path == "/agent?artifact="+id {
			// This fixture has no model API key; dismiss the agent's independent
			// first-use dialog before interacting with the preview's notice.
			run(chromedp.Poll[chromedp.Void](`!document.getElementById('key-modal').hidden`),
				chromedp.Click(chromedp.CSS(`#key-modal button[onclick="closeKeyModal()"]`)))
		}
		check(`document.getElementById('net-title').innerText === 'Connection data protection'`)
		check(`document.getElementById('net-warning-list').innerText.includes('Your origin approval is saved.')`)
		check(`getComputedStyle(document.getElementById('net-allow')).display === 'none' && getComputedStyle(document.getElementById('net-never')).display === 'none'`)
		check(`document.getElementById('net-once').innerText === 'Close'`)
		check(`document.querySelector('#net-warning-list a').getAttribute('href') === '/connections?artifact=` + id + `'`)
		run(chromedp.Click(chromedp.CSS("#net-once")))
		run(chromedp.Poll[chromedp.Void](`document.getElementById('net-modal').hidden`))
		assert.Zero(t, stylesheetCalls.Load(), "the explanation must not weaken the actual CSP: %s", path)
	}
	after, err := ro.cfg.Store.ListOriginDecisions(t.Context(), 1, id)
	require.NoError(t, err)
	assert.Equal(t, before, after, "reading and closing the explanation preserves the saved approval")

	// The same prompt still offers a working approval when no connection data
	// restricts the destination. The stylesheet loads only after that approval.
	ordinaryID := createArtifact(t, ro, map[string]any{
		"title": "Ordinary app", "body": body, "network_allowlist": []string{},
	})
	run(chromedp.Navigate(app.URL+"/artifacts/"+ordinaryID), chromedp.Poll[chromedp.Void](`
!document.getElementById('net-modal')?.hidden && !!document.querySelector('#net-warning-list [data-network-origin-reviewed]')`))
	check(`document.getElementById('net-title').innerText === 'Allow network access?'`)
	check(`getComputedStyle(document.getElementById('net-allow')).display !== 'none' && getComputedStyle(document.getElementById('net-never')).display !== 'none'`)
	check(`document.getElementById('net-once').innerText === 'Block once'`)
	assert.Zero(t, stylesheetCalls.Load())
	run(chromedp.Click(chromedp.CSS("#net-allow")), chromedp.Poll[chromedp.Void](`document.getElementById('net-modal').hidden`))
	require.Eventually(t, func() bool { return stylesheetCalls.Load() > 0 }, 5*time.Second, 10*time.Millisecond)
	allowed, err := ro.cfg.Store.AllowedOrigins(t.Context(), 1, ordinaryID)
	require.NoError(t, err)
	assert.Contains(t, allowed, cdn.URL)
}
