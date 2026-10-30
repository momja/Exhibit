package api

import (
	"context"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/chromedp"
	"github.com/momja/Exhibit/internal/chromium"
	"github.com/momja/Exhibit/internal/chromium/chromiumtest"
	"github.com/momja/Exhibit/internal/connectors"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/require"
)

func newPersonalConnectionsFixture(t *testing.T) (*Router, string, string) {
	t.Helper()
	ro := newTestRouter(t)
	ro.cfg.Relay.Enabled = true
	st := ro.cfg.Store.(*store.SQLiteStore)
	ro.cfg.Connections = connectors.New(connectors.Config{
		Store: st, Box: ro.cfg.Secrets, CustomEnabled: true, OAuthEnabled: true,
		GitHub: connectors.OAuthProvider{ClientID: "screenshot-fixture", ClientSecret: "fixture-only"},
	})
	ro.cfg.ConnectionPolicy = ro.cfg.Connections
	id := createArtifact(t, ro, map[string]any{
		"title": "My reading list", "body": "<!doctype html><title>My reading list</title><h1>My reading list</h1><p>Books and articles to read next.</p>",
	})
	ctx := t.Context()
	const origin = "https://reading.example.test"
	require.NoError(t, st.SetOriginDecision(ctx, 1, id, origin, store.DecisionAllow, "test"))
	account, err := ro.cfg.Connections.Create(ctx, 1, connectors.CreateRequest{Name: "My reading account", Recipe: "bearer", Origin: origin, Secret: "fixture-original"}, connectors.UnlimitedLimits())
	require.NoError(t, err)
	_, err = ro.cfg.Connections.Grant(ctx, 1, id, connectors.GrantRequest{ConnectionID: account.ID})
	require.NoError(t, err)
	_, err = st.RequestArtifactConnection(ctx, 1, id, "github", "Bring your saved repositories into this reading list.")
	require.NoError(t, err)
	require.NoError(t, st.SetStateAsViewer(ctx, 1, id, "reading-list", `[{"title":"A guide to better notes","status":"To read"}]`))
	return ro, id, account.ID
}

func TestPersonalConnectionsBrowser(t *testing.T) {
	for _, width := range []int64{1280, 390} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			ro, id, connectionID := newPersonalConnectionsFixture(t)
			app := httptest.NewServer(ro)
			defer app.Close()
			ro.cfg.AppOrigin = app.URL
			browser, err := chromium.New(chromiumtest.Start(t))
			require.NoError(t, err)
			ctx, cancel, err := browser.Tab(context.Background())
			require.NoError(t, err)
			defer cancel()
			ctx, cancel = context.WithTimeout(ctx, 45*time.Second)
			defer cancel()
			_, err = chromedp.Call(ctx, emulation.SetDeviceMetricsOverride, emulation.SetDeviceMetricsOverrideParams{Width: width, Height: 900, DeviceScaleFactor: 1, Mobile: width < 600})
			require.NoError(t, err)
			check := func(js, message string) {
				t.Helper()
				ok, err := chromedp.Run(ctx, chromedp.Evaluate[bool](js))
				require.NoError(t, err)
				require.True(t, ok, message)
			}
			base := app.URL + "/connections?artifact=" + id
			require.NoError(t, chromedp.Do(ctx, chromedp.Navigate(base), chromedp.Poll[chromedp.Void](`document.querySelector('.connect-submit')?.textContent === 'Continue to GitHub'`)))
			check(`!document.querySelector('#connection-origin, #connection-secret, #connection-header, [name="share_with_recipients"]') && Array.from(document.querySelector('#connection-kind').options).every(o => o.value === 'github')`, "ordinary setup contains named services and no custom credential or account-sharing inputs")
			check(`document.querySelector('.connection-grant').textContent.includes('Allowed for your account only.') && document.body.textContent.includes('Saved results are separate') && document.documentElement.scrollWidth <= innerWidth`, "personal permission and saved results are distinct, with no horizontal overflow")
			check(fmt.Sprintf(`(() => { const link = document.querySelector('.connect-request a'); const url = new URL(link.href); return link.textContent === 'Connect account' && url.searchParams.get('artifact') === %q && url.searchParams.get('provider') === 'github'; })()`, id), "agent request offers a concrete setup action tied to the tool")
			_, err = chromedp.RunResponse(ctx, chromedp.Click(chromedp.CSS(`.connect-request a`)))
			require.NoError(t, err)
			check(fmt.Sprintf(`new URLSearchParams(location.search).get('artifact') === %q && document.querySelector('.connect-artifact-context h3').textContent === 'My reading list'`, id), "request navigation preserves the originating tool")

			require.NoError(t, chromedp.Do(ctx, chromedp.Navigate(base+"&advanced=1&provider=bearer"), chromedp.Poll[chromedp.Void](`document.querySelector('#connection-secret')?.disabled === false`)))
			check(`document.querySelector('.connect-lede h2').textContent === 'Advanced connections' && !document.querySelector('#connection-origin').disabled && document.querySelector('#connection-secret').type === 'password' && document.documentElement.scrollWidth <= innerWidth`, "advanced custom setup explicitly exposes credential fields and fits mobile")

			before, err := ro.cfg.Store.(*store.SQLiteStore).GetConnection(ctx, 1, connectionID)
			require.NoError(t, err)
			require.NoError(t, chromedp.Do(ctx, chromedp.Navigate(base+"&reconnect="+connectionID), chromedp.WaitVisible(chromedp.CSS(`#connection-setup a.btn`))))
			check(`!document.querySelector('#repair-secret') && document.querySelector('#connection-setup a.btn').textContent === 'Open advanced reconnect'`, "custom repair credentials are confined to advanced setup")
			_, err = chromedp.RunResponse(ctx, chromedp.Click(chromedp.CSS(`#connection-setup a.btn`)))
			require.NoError(t, err)
			require.NoError(t, chromedp.Do(ctx,
				chromedp.WaitVisible(chromedp.CSS(`#repair-secret`)),
				chromedp.Evaluate[chromedp.Void](fmt.Sprintf(`import(%q).then(({application}) => { window.connectionsApplication = application; })`, assetURL("gallery/controllers.js"))),
				chromedp.Poll[chromedp.Void](`window.connectionsApplication?.getControllerForElementAndIdentifier(document.querySelector('.connect-page'), 'connections')?.active && document.getAnimations().length === 0`),
				chromedp.SendKeys(chromedp.CSS(`#repair-secret`), "fixture-replacement")))
			check(`!document.querySelector('[data-connections-target="createForm"]') && document.querySelector('#repair-secret').value === 'fixture-replacement' && document.documentElement.scrollWidth <= innerWidth`, "reconnect is a repair form and fits the viewport")
			_, err = chromedp.RunResponse(ctx, chromedp.Click(chromedp.CSS(`[data-action="submit->connections#repair"] button`)))
			require.NoError(t, err)
			require.NoError(t, chromedp.Do(ctx, chromedp.WaitVisible(chromedp.CSS(`.connection-grant`))))
			check(fmt.Sprintf(`new URLSearchParams(location.search).get('artifact') === %q && new URLSearchParams(location.search).get('connection') === %q && document.querySelectorAll('.connection-grant').length === 1`, id, connectionID), "repair returns to this tool with its original account and grant")
			after, err := ro.cfg.Store.(*store.SQLiteStore).GetConnection(ctx, 1, connectionID)
			require.NoError(t, err)
			require.NotEqual(t, before.EncryptedSecret, after.EncryptedSecret)
			grants, err := ro.cfg.Connections.Grants(ctx, 1, id)
			require.NoError(t, err)
			require.Len(t, grants, 1)
			require.Equal(t, connectionID, grants[0].Connection.ID)
		})
	}
}
