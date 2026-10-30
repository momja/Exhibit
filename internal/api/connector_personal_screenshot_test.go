//go:build screenshot

package api

import (
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/chromedp"
	"github.com/momja/Exhibit/internal/chromium"
	"github.com/momja/Exhibit/internal/chromium/chromiumtest"
	"github.com/stretchr/testify/require"
)

// TestCapturePersonalConnections captures real server templates and production
// assets. Only accounts and saved data are fixtures; no external provider call
// is needed. Run with: go test -tags screenshot ./internal/api -run '^TestCapturePersonalConnections$' -count=1.
func TestCapturePersonalConnections(t *testing.T) {
	ro, id, connectionID := newPersonalConnectionsFixture(t)
	app := httptest.NewServer(ro)
	defer app.Close()
	ro.cfg.AppOrigin = app.URL
	render := httptest.NewServer(ro.RenderHandler())
	defer render.Close()
	ro.cfg.RenderOrigin = render.URL
	browser, err := chromium.New(chromiumtest.Start(t))
	require.NoError(t, err)
	ctx, cancel, err := browser.Tab(context.Background())
	require.NoError(t, err)
	defer cancel()
	ctx, cancel = context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	out := "../../docs/screenshots/Exh-k6gt"
	require.NoError(t, os.MkdirAll(out, 0755))
	viewport := func(width, height int64, dark bool) {
		t.Helper()
		_, err := chromedp.Call(ctx, emulation.SetDeviceMetricsOverride, emulation.SetDeviceMetricsOverrideParams{Width: width, Height: height, DeviceScaleFactor: 1, Mobile: width < 600})
		require.NoError(t, err)
		scheme := "light"
		if dark {
			scheme = "dark"
		}
		_, err = chromedp.Call(ctx, emulation.SetEmulatedMedia, emulation.SetEmulatedMediaParams{Features: []*emulation.MediaFeature{{Name: "prefers-color-scheme", Value: scheme}, {Name: "prefers-reduced-motion", Value: "reduce"}}})
		require.NoError(t, err)
	}
	capture := func(name, selector string, aliases ...string) {
		t.Helper()
		require.NoError(t, chromedp.Do(ctx, chromedp.Poll[chromedp.Void](`document.fonts.status === 'loaded' && document.getAnimations().length === 0`)))
		shot, err := chromedp.Run(ctx, chromedp.Screenshot(chromedp.CSS(selector)))
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(out, name), shot, 0644))
		for _, alias := range aliases {
			require.NoError(t, os.WriteFile(filepath.Join("../../docs/screenshots", alias), shot, 0644))
		}
	}
	base := app.URL + "/connections?artifact=" + id
	viewport(1280, 1000, false)
	require.NoError(t, chromedp.Do(ctx, chromedp.Navigate(base+"&connection="+connectionID), chromedp.Poll[chromedp.Void](`document.querySelector('.connect-submit')?.textContent === 'Continue to GitHub'`)))
	capture("01-personal-connections-desktop.png", "body", "Exh-g2vv/01-connections-desktop.png", "Exh-s47o/02-additive-connection-access-desktop.png")
	viewport(390, 844, true)
	capture("02-personal-connections-mobile-dark.png", "body", "Exh-g2vv/02-connections-mobile-dark.png", "Exh-s47o/03-additive-connection-access-mobile.png")

	viewport(1280, 1000, false)
	require.NoError(t, chromedp.Do(ctx, chromedp.Navigate(base+"&advanced=1&provider=bearer"), chromedp.Poll[chromedp.Void](`document.querySelector('#connection-secret')?.disabled === false`)))
	capture("03-advanced-custom-setup.png", "body")
	require.NoError(t, chromedp.Do(ctx, chromedp.Navigate(base+"&advanced=1&reconnect="+connectionID), chromedp.WaitVisible(chromedp.CSS(`#repair-secret`))))
	capture("04-reconnect-existing-account.png", "body")

	require.NoError(t, chromedp.Do(ctx, chromedp.Navigate(app.URL+"/artifacts/"+id), chromedp.WaitVisible(chromedp.CSS(`#share-open`)), chromedp.Click(chromedp.CSS(`#share-open`)), chromedp.WaitVisible(chromedp.CSS(`#share-panel`))))
	capture("05-sharing-saved-results.png", ".share-modal")
	require.NoError(t, chromedp.Do(ctx,
		chromedp.Click(chromedp.CSS(`#share-close`)),
		chromedp.Evaluate[chromedp.Void](fmt.Sprintf(`import(%q).then(({application}) => { window.relayController = application.getControllerForElementAndIdentifier(document.body, 'relay-host'); })`, assetURL("gallery/controllers.js"))),
		chromedp.Poll[chromedp.Void](`window.relayController?.active`),
		chromedp.Evaluate[chromedp.Void](`window.relayController.askConsent({url:'https://reading.example.test/items'}, new AbortController().signal)`),
		chromedp.WaitVisible(chromedp.CSS(`.relay-consent[open]`))))
	capture("06-relay-consent.png", ".relay-consent", "Exh-g2vv/06-relay-consent.png")
}
