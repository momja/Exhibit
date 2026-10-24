package api

import (
	"context"
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

func TestConnectionPermissionsBrowserSavesRefreshesAndCancels(t *testing.T) {
	browser, err := chromium.New(chromiumtest.Start(t))
	require.NoError(t, err)
	ctx, cancel, err := browser.Tab(context.Background())
	require.NoError(t, err)
	defer cancel()
	ctx, timeout := context.WithTimeout(ctx, 40*time.Second)
	defer timeout()

	ro := newTestRouter(t)
	ro.cfg.Relay.Enabled = true
	ro.cfg.Connections = connectors.New(connectors.Config{Store: ro.cfg.Store.(*store.SQLiteStore), Box: ro.cfg.Secrets})
	ro.cfg.ConnectionPolicy = ro.cfg.Connections
	c, err := ro.cfg.Connections.Create(t.Context(), 1, connectors.CreateRequest{
		Name: "Fixture API", Recipe: "bearer", Origin: "https://api.example.test", Secret: "fixture-only-credential",
	}, connectors.UnlimitedLimits())
	require.NoError(t, err)
	before, err := ro.cfg.Store.(*store.SQLiteStore).GetConnection(t.Context(), 1, c.ID)
	require.NoError(t, err)

	var updates atomic.Int64
	mux := http.NewServeMux()
	app := httptest.NewServer(mux)
	defer app.Close()
	ro.cfg.AppOrigin = app.URL
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch && r.URL.Path == "/api/connections/"+c.ID {
			updates.Add(1)
		}
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
	form := `.connection-permissions-form[data-connection-id="` + c.ID + `"]`
	checkbox := form + ` input[name="allow_writes"]`
	openEditor := chromedp.Click(chromedp.CSS(`summary[aria-label="Edit permissions for Fixture API"]`))
	save := chromedp.Click(chromedp.CSS(form + ` button[type="submit"]`))
	toggle := chromedp.Click(chromedp.CSS(checkbox))
	assertWrites := func(want bool) {
		t.Helper()
		list, err := ro.cfg.Connections.List(t.Context(), 1)
		require.NoError(t, err)
		require.Len(t, list, 1)
		if want {
			assert.Contains(t, list[0].Capabilities, "write")
		} else {
			assert.NotContains(t, list[0].Capabilities, "write")
		}
	}

	run(chromedp.Navigate(app.URL+"/connections"),
		chromedp.Poll[chromedp.Void](`document.querySelector('[data-recipes="basic"]')?.hidden`))
	check(`!document.querySelector('` + checkbox + `').checked`)
	run(openEditor, toggle, save,
		// defaultChecked changes only when htmx replaces the card with the
		// server-rendered permission, not when the local checkbox is toggled.
		chromedp.Poll[chromedp.Void](`document.querySelector('`+checkbox+`')?.defaultChecked === true`))
	check(`!document.querySelector('` + form + `').closest('details').open`)
	check(`document.querySelector('[data-connections-target="feedback"]').textContent.includes('Reopen affected artifacts')`)
	assertWrites(true)
	assert.EqualValues(t, 1, updates.Load())

	run(openEditor, toggle, chromedp.Click(chromedp.CSS(form+` button[type="button"]`)))
	check(`document.querySelector('` + checkbox + `').checked && !document.querySelector('` + form + `').closest('details').open`)
	assert.EqualValues(t, 1, updates.Load(), "Cancel sends no update")
	assertWrites(true)

	run(openEditor, toggle, save,
		chromedp.Poll[chromedp.Void](`document.querySelector('`+checkbox+`')?.defaultChecked === false`))
	assert.EqualValues(t, 2, updates.Load())
	assertWrites(false)
	after, err := ro.cfg.Store.(*store.SQLiteStore).GetConnection(t.Context(), 1, c.ID)
	require.NoError(t, err)
	assert.Equal(t, before.EncryptedSecret, after.EncryptedSecret)

	// A stale editor cannot revive a revoked connection. Preserve the edit and
	// show its failure inline instead of replacing the form or reporting success.
	require.NoError(t, ro.cfg.Connections.Revoke(t.Context(), 1, c.ID))
	run(openEditor, toggle, save,
		chromedp.Poll[chromedp.Void](`!!document.querySelector('`+form+` [data-connection-permissions-error]')?.textContent`))
	check(`document.querySelector('` + form + `').closest('details').open && document.querySelector('` + checkbox + `').checked`)
	assert.EqualValues(t, 3, updates.Load())
}
