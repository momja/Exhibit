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

func TestAPIConnectorApprovalBrowser(t *testing.T) {
	h := newPiHarness(t)
	app := httptest.NewServer(h.router)
	browser, err := chromium.New(chromiumtest.Start(t))
	require.NoError(t, err)
	ctx, closeTab, err := browser.Tab(context.Background())
	require.NoError(t, err)
	ctx, timeout := context.WithTimeout(ctx, 120*time.Second)
	defer func() { timeout(); closeTab(); app.CloseClientConnections(); app.Close() }()
	run := func(actions ...chromedp.Action[chromedp.Void]) {
		t.Helper()
		require.NoError(t, chromedp.Do(ctx, actions...))
	}
	check := func(js string) {
		t.Helper()
		ok, err := chromedp.Run(ctx, chromedp.Evaluate[bool](js))
		require.NoError(t, err)
		require.True(t, ok, js)
	}
	for _, width := range []int64{1280, 390, 320} {
		theme := "light"
		if width == 390 {
			theme = "dark"
		}
		_, err = chromedp.Call(ctx, emulation.SetEmulatedMedia, emulation.SetEmulatedMediaParams{Features: []*emulation.MediaFeature{{Name: "prefers-color-scheme", Value: theme}}})
		require.NoError(t, err)
		session, artifact, _, _, unsubscribe := askMockAPIConnector(t, h)
		run(chromedp.EmulateViewport(width, 1000), chromedp.Navigate(app.URL+"/agent?artifact="+artifact+"&live="+session), chromedp.Poll[chromedp.Void](`!!document.querySelector('[data-controller="agent-api-connector"]')`))
		check(`document.querySelector('.connector-trust input').checked === false`)
		check(`document.querySelector('.connector-key input').type === 'password' && document.querySelector('.connector-key input').value === ''`)
		run(chromedp.Click(chromedp.CSS(`[data-controller="agent-api-connector"] [type="submit"]`)))
		check(`document.querySelector('[data-interaction-id]').dataset.pending === 'true'`)
		run(chromedp.SendKeys(chromedp.CSS(`.connector-key input`), "browser-private-key"), chromedp.Click(chromedp.CSS(`[data-controller="agent-api-connector"] [type="submit"]`)))
		check(`document.querySelector('[data-interaction-id]').dataset.pending === 'true'`)
		check(`document.documentElement.scrollWidth <= innerWidth`)
		if dir := os.Getenv("EXHIBIT_REVIEW_DIR"); dir != "" {
			require.NoError(t, os.MkdirAll(dir, 0755))
			run(chromedp.Evaluate[chromedp.Void](`document.querySelector('[data-interaction-id]').scrollIntoView({block:'start'})`))
			shot, err := chromedp.Run(ctx, chromedp.CaptureScreenshot())
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(dir, fmt.Sprintf("connector-%d.png", width)), shot, 0644))
		}
		if width == 1280 {
			run(chromedp.Evaluate[chromedp.Void](`document.querySelector('.connector-key input').value = 'sk-proj-fake-wrong-domain'`),
				chromedp.Click(chromedp.CSS(`.connector-trust input`)),
				chromedp.Click(chromedp.CSS(`[data-controller="agent-api-connector"] [type="submit"]`)),
				chromedp.Poll[chromedp.Void](`document.querySelector('.interaction-status')?.textContent.includes('normally goes to') && document.querySelector('.connector-key input')?.value === '' && !document.querySelector('.connector-trust input')?.checked`))
			check(`document.querySelector('[data-interaction-id]').dataset.pending === 'true' && !document.querySelector('.connector-trust input').checked`)
			run(chromedp.SendKeys(chromedp.CSS(`.connector-key input`), "browser-private-key"))
			run(chromedp.Click(chromedp.CSS(`.connector-trust input`)), chromedp.Click(chromedp.CSS(`[data-controller="agent-api-connector"] [type="submit"]`)), chromedp.Poll[chromedp.Void](`!document.querySelector('.connector-key') && document.querySelector('.interaction-status')?.textContent.includes('Connection ready')`))
		} else {
			run(chromedp.Click(chromedp.CSS(`[data-action="agent-api-connector#deny"]`)), chromedp.Poll[chromedp.Void](`!document.querySelector('.connector-key') && document.querySelector('.interaction-status')?.textContent.includes('Connection declined')`))
		}
		check(`!document.querySelector('input[type="password"]') || document.querySelector('input[type="password"]').value === ''`)

		connections, err := h.router.cfg.Connections.List(context.Background(), defaultOwnerID)
		require.NoError(t, err)
		for _, c := range connections {
			require.NoError(t, h.router.cfg.Connections.Revoke(context.Background(), defaultOwnerID, c.ID))
		}
		unsubscribe()
	}
}
