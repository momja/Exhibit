package api

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/chromedp"
	"github.com/momja/Exhibit/internal/chromium"
	"github.com/momja/Exhibit/internal/chromium/chromiumtest"
	"github.com/stretchr/testify/require"
)

func TestSearchTabBrowser(t *testing.T) {
	app, in := galleryNavigationApp(t)
	require.NoError(t, in.ro.cfg.Store.UpdateArtifact(t.Context(), in.ownerTwo, in.artifactTwo, map[string]any{"title": "Owned chart"}))
	browser, err := chromium.New(chromiumtest.Start(t))
	require.NoError(t, err)
	for _, width := range []int{320, 390, 767, 1280} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			ctx, cancel, err := browser.Tab(context.Background())
			require.NoError(t, err)
			defer cancel()
			ctx, cancel = context.WithTimeout(ctx, 40*time.Second)
			defer cancel()
			run := func(js string) {
				t.Helper()
				require.NoError(t, chromedp.Do(ctx, chromedp.Evaluate[chromedp.Void](js)))
			}
			poll := func(js string) {
				t.Helper()
				require.NoError(t, chromedp.Do(ctx, chromedp.Poll[chromedp.Void](js, chromedp.WithPollingTimeout(6*time.Second))), js)
			}
			check := func(js string) {
				t.Helper()
				ok, err := chromedp.Run(ctx, chromedp.Evaluate[bool]("Boolean("+js+")"))
				require.NoError(t, err)
				require.True(t, ok, js)
			}
			_, err = chromedp.Call(ctx, emulation.SetDeviceMetricsOverride, emulation.SetDeviceMetricsOverrideParams{Width: int64(width), Height: 844, DeviceScaleFactor: 1, Mobile: width < 768})
			require.NoError(t, err)
			require.NoError(t, chromedp.Do(ctx, chromedp.Navigate(app.URL+"/")))
			poll(`window.clerkCalls?.mount === 1`)
			run(`window.originalDocument=document;window.originalHeader=document.querySelector('header');window.originalAccount=document.querySelector('[data-clerk-target="mount"] button')`)
			if width < 768 {
				check(`!document.querySelector('header [data-gallery-query]').getClientRects().length && document.querySelector('.gallery-spaces li:nth-child(2) a').getAttribute('href') === '/search' && document.querySelector('.gallery-spaces a[href="/search"]').getClientRects().length > 0`)
			} else {
				check(`document.querySelector('header [data-gallery-query]').getClientRects().length > 0 && !document.querySelector('.gallery-spaces a[href="/search"]').getClientRects().length`)
			}
			if width == 390 {
				run(`document.body.dispatchEvent(new KeyboardEvent('keydown',{key:'/',bubbles:true}))`)
			} else {
				run(`document.querySelector('.gallery-spaces a[href="/search"]').click()`)
			}
			poll(`location.pathname === '/search' && document.querySelector('.gallery-page-search [data-gallery-query]')`)
			check(`document === originalDocument && document.querySelector('header') === originalHeader && !document.querySelector('header [data-gallery-query]') && document.querySelector('.gallery-spaces [aria-current]').getAttribute('href') === '/search' && !document.querySelector('.card') && document.activeElement.matches('[data-gallery-query]')`)
			run(`document.querySelector('[data-gallery-query]').value='chart';document.querySelector('[data-gallery-query]').dispatchEvent(new Event('input',{bubbles:true}))`)
			poll(`location.search === '?q=chart' && document.querySelectorAll('.card').length >= 24`)
			run(`document.querySelector('[data-gallery-page]')?.click()`)
			poll(`document.querySelectorAll('.card').length === 31`)
			check(`document.querySelector('[data-href="/artifacts/owned-tool"] .card-actions') && document.querySelector('.card-by') && !document.querySelector('.card-by').closest('.card').querySelector('.card-actions, .capability-cluster') && document.documentElement.scrollWidth <= innerWidth`)
			// Navigate away/back through htmx; the query and correct selection return.
			run(`document.querySelector('.gallery-spaces a[href="/shared"]').click()`)
			poll(`location.pathname === '/shared' && document.querySelector('#shared-gallery')`)
			run(`history.back()`)
			poll(`location.pathname === '/search' && document.querySelector('[data-gallery-query]')?.value === 'chart' && document.querySelector('[data-href="/artifacts/owned-tool"]')`)
			check(`document.querySelectorAll('[data-gallery-search]').length === 1 && document.querySelector('[data-gallery-clear]').hidden === false`)
			run(`document.querySelector('[data-gallery-clear]').click()`)
			poll(`location.search === '' && !document.querySelector('.card') && document.querySelector('.empty')?.textContent === 'Search your library and artifacts shared with you.'`)
			check(`document.activeElement.matches('[data-gallery-query]')`)
			run(`document.querySelector('[data-gallery-query]').value='missing';document.querySelector('[data-gallery-query]').dispatchEvent(new Event('input',{bubbles:true}));document.querySelector('[data-gallery-search]').requestSubmit()`)
			// Submit also works without waiting for the typing debounce.
			poll(`document.querySelector('.empty')?.textContent === 'No artifacts match your search.'`)
			run(`history.forward()`)
			poll(`location.pathname === '/shared' && document.querySelector('#shared-gallery')`)
			check(`document === originalDocument && document.querySelector('header') === originalHeader && document.querySelector('[data-clerk-target="mount"] button') === originalAccount && clerkCalls.mount === 1 && clerkCalls.unmount === 0 && document.documentElement.scrollWidth <= innerWidth`)
		})
	}
}

func TestSearchPageWithoutScriptsBrowser(t *testing.T) {
	app, _ := galleryNavigationApp(t, true)
	browser, err := chromium.New(chromiumtest.Start(t))
	require.NoError(t, err)
	ctx, cancel, err := browser.Tab(context.Background())
	require.NoError(t, err)
	defer cancel()
	ctx, cancel = context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	require.NoError(t, chromedp.Do(ctx, chromedp.Navigate(app.URL+"/search?q=chart")))
	ok, err := chromedp.Run(ctx, chromedp.Evaluate[bool](`document.querySelector('[data-gallery-query]').value === 'chart' && document.querySelectorAll('.card').length === 24 && !window.Clerk`))
	require.NoError(t, err)
	require.True(t, ok)
	require.NoError(t, chromedp.Do(ctx, chromedp.Evaluate[chromedp.Void](`document.querySelector('[data-gallery-query]').value='Incoming';document.querySelector('[data-gallery-search]').requestSubmit()`)))
	// Native form navigation replaces the execution context. Inspect the new
	// document on each retry rather than polling the departing document.
	require.Eventually(t, func() bool {
		ok, err := chromedp.Run(ctx, chromedp.Evaluate[bool](`location.pathname === '/search' && location.search === '?q=Incoming' && document.querySelectorAll('.card').length === 1 && document.querySelector('[data-href="/artifacts/incoming-tool"]') !== null`))
		return err == nil && ok
	}, 5*time.Second, 50*time.Millisecond)
}
