package api

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/momja/Exhibit/internal/chromium"
	"github.com/momja/Exhibit/internal/chromium/chromiumtest"
	"github.com/momja/Exhibit/internal/store"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/chromedp"
	"github.com/stretchr/testify/require"
)

func TestGalleryRedesignBrowser(t *testing.T) {
	app, in := galleryNavigationApp(t)
	require.NoError(t, in.ro.cfg.Store.SaveTranscript(t.Context(), in.ownerTwo, store.Transcript{ArtifactID: in.artifactTwo, SessionID: "color-chat", Title: "Make the chart green", SessionFile: piFile}))
	source := "activity-body"
	require.NoError(t, in.ro.cfg.Blob.Put(t.Context(), source, strings.NewReader("<h1>Owned tool</h1>")))
	_, err := in.ro.cfg.Store.CommitVersion(t.Context(), in.ownerTwo, in.artifactTwo, store.VersionChange{Provenance: store.Provenance{Origin: store.VersionAgent, Message: "Make the chart green", SessionID: "color-chat"}, BodyBlobID: &source})
	require.NoError(t, err)
	browser, err := chromium.New(chromiumtest.Start(t))
	require.NoError(t, err)
	for _, width := range []int{320, 390, 1280} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			require.NoError(t, in.ro.cfg.Store.SetArtifactStarred(t.Context(), store.ViewerID(in.ownerTwo), in.artifactTwo, false))
			ctx, cancel, err := browser.Tab(context.Background())
			require.NoError(t, err)
			defer cancel()
			ctx, cancel = context.WithTimeout(ctx, 45*time.Second)
			defer cancel()
			run := func(js string) {
				t.Helper()
				require.NoError(t, chromedp.Do(ctx, chromedp.Evaluate[chromedp.Void](js)))
			}
			poll := func(js string) {
				t.Helper()
				err := chromedp.Do(ctx, chromedp.Poll[chromedp.Void](js, chromedp.WithPollingTimeout(7*time.Second)))
				if err != nil {
					state, _ := chromedp.Run(ctx, chromedp.Evaluate[string](`JSON.stringify({url:location.href,body:document.body.textContent.slice(0,800),href:document.querySelector('.card-title')?.href})`))
					t.Log(state)
				}
				require.NoError(t, err, js)
			}
			_, err = chromedp.Call(ctx, emulation.SetDeviceMetricsOverride, emulation.SetDeviceMetricsOverrideParams{Width: int64(width), Height: 844, DeviceScaleFactor: 1, Mobile: width < 768})
			require.NoError(t, err)
			require.NoError(t, chromedp.Do(ctx, chromedp.Navigate(app.URL+"/search?q=Owned"), chromedp.WaitVisible(chromedp.CSS(`#search-gallery`))))
			poll(`document.querySelector('[data-href="/artifacts/owned-tool"] [data-action-menu-target="trigger"]') && window.clerkCalls?.mount===1`)
			run(`window.originalHeader=document.querySelector('header');window.originalDocument=document;document.querySelector('[data-href="/artifacts/owned-tool"] .card-actions-trigger').click()`)
			poll(`document.querySelector('.card-actions-menu:not([hidden])')`)
			run(`document.querySelector('.card-actions-menu:not([hidden]) [data-action-menu-event-param="exhibit:star"]').click()`)
			poll(`document.querySelector('.card-star') && location.search==='?q=Owned'`)
			run(`document.querySelector('.gallery-spaces a[href="/"]').click()`)
			poll(`location.pathname==='/' && document.querySelectorAll('.card').length===1`)
			poll(`!document.querySelector('.gallery-filter-trigger') && ![...document.querySelectorAll('[data-gallery-query]')].some(e=>e.getClientRects().length) && document.querySelectorAll('.gallery-spaces a').length===4`)
			require.NoError(t, chromedp.Do(ctx, chromedp.Click(chromedp.CSS(`.card-title`)), chromedp.WaitReady(chromedp.CSS(`#pip-title`))))
			// Real viewer load records the opening through the personal API.
			require.Eventually(t, func() bool {
				prefs, err := in.ro.cfg.Store.ArtifactPreferences(t.Context(), store.ViewerID(in.ownerTwo), []string{in.artifactTwo})
				return err == nil && !prefs[in.artifactTwo].LastOpenedAt.IsZero()
			}, 5*time.Second, 25*time.Millisecond)
			require.NoError(t, chromedp.Do(ctx, chromedp.Navigate(app.URL+"/"), chromedp.WaitVisible(chromedp.CSS(`#starred-gallery .card-title`))))
			poll(`document.querySelector('[data-controller="action-menu"]')`)
			run(`document.querySelector('.card-actions-trigger').click()`)
			poll(`document.querySelector('.card-actions-menu:not([hidden])')`)
			run(`document.querySelector('.card-actions-menu:not([hidden]) [data-action-menu-event-param="exhibit:star"]').click()`)
			poll(`!document.querySelector('.card') && document.querySelector('.empty')`)
			run(`document.querySelector('.gallery-spaces a[href="/activity"]').click()`)
			poll(`location.pathname==='/activity' && document.querySelector('.activity-item[href*="conversation=color-chat"]') && document.documentElement.scrollWidth<=innerWidth`)
			require.NoError(t, chromedp.Do(ctx, chromedp.Click(chromedp.CSS(`.activity-item[href*="conversation=color-chat"]`)), chromedp.WaitReady(chromedp.CSS(`#history`))))
			poll(`location.pathname==='/agent' && document.querySelector('.chat.history-open') && document.querySelector('#history').textContent.includes('Done.') && !document.querySelector('dialog[open]')`)
		})
	}
}
