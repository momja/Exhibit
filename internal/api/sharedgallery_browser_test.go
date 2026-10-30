package api

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/momja/Exhibit/internal/chromium"
	"github.com/momja/Exhibit/internal/chromium/chromiumtest"
	"github.com/stretchr/testify/require"
)

func TestSharedGalleryRecipientBrowser(t *testing.T) {
	browser, err := chromium.New(chromiumtest.Start(t))
	require.NoError(t, err)
	ctx, cancel, err := browser.Tab(context.Background())
	require.NoError(t, err)
	defer cancel()
	ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	in := newGrantedInstance(t)
	app := httptest.NewServer(in.ro)
	defer app.Close()
	in.ro.cfg.AppOrigin = app.URL
	_, err = chromedp.Call(ctx, network.SetCookie, network.SetCookieParams{Name: in.cookieTwo.Name, Value: in.cookieTwo.Value, URL: app.URL, Path: "/"})
	require.NoError(t, err)

	require.NoError(t, chromedp.Do(ctx,
		chromedp.Navigate(app.URL+"/shared"),
		chromedp.WaitVisible(chromedp.CSS(`.card-title`)),
	))
	correct, err := chromedp.Run(ctx, chromedp.Evaluate[bool](`document.querySelectorAll('.card').length === 1 && document.querySelector('.card-owner-name').textContent === 'one@example.test' && !document.querySelector('.card-actions, .capability-cluster, [data-duplicate-dialog]')`))
	require.NoError(t, err)
	require.True(t, correct)
	require.NoError(t, chromedp.Do(ctx,
		chromedp.Click(chromedp.CSS(`.card-title`)),
		chromedp.WaitReady(chromedp.CSS(`#pip-title`)),
	))
	correct, err = chromedp.Run(ctx, chromedp.Evaluate[bool](`!document.querySelector('a[href$="/edit"]') && document.body.textContent.includes('`+ownerOneTitle+`')`))
	require.NoError(t, err)
	require.True(t, correct, "recipient opens the ordinary read-only detail page")
	require.NoError(t, in.ro.cfg.Store.DeleteGrant(context.Background(), in.ownerOne, in.granted, in.ownerTwo))
	require.NoError(t, chromedp.Do(ctx,
		chromedp.Navigate(app.URL+"/shared"),
		chromedp.Poll[chromedp.Void](`document.querySelectorAll('.card').length === 0 && document.body.textContent.includes('Nothing has been shared with you yet.')`),
		chromedp.Navigate(app.URL+"/artifacts/"+in.granted),
		chromedp.Poll[chromedp.Void](`!document.querySelector('iframe') && !document.body.textContent.includes('`+ownerOneTitle+`')`),
	))
}
