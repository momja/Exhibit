package api

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
	"github.com/momja/Exhibit/internal/chromium"
	"github.com/momja/Exhibit/internal/chromium/chromiumtest"
	"github.com/stretchr/testify/require"
)

func TestGalleryFilterBrowser(t *testing.T) {
	for _, phone := range []bool{true, false} {
		name := "desktop"
		if phone {
			name = "phone"
		}
		t.Run(name, func(t *testing.T) {
			ro := newTestRouter(t)
			seedGalleryArtifacts(t, ro, 2)
			seedGalleryFilters(t, ro, defaultOwnerID)
			require.NoError(t, ro.cfg.Store.AddArtifactTag(context.Background(), defaultOwnerID, "gallery-000", "tag-1-0"))
			require.NoError(t, ro.cfg.Store.AddArtifactToCollection(context.Background(), defaultOwnerID, "gallery-000", "collection-1-0"))
			app := httptest.NewServer(ro)
			defer app.Close()
			ro.cfg.AppOrigin = app.URL
			browser, err := chromium.New(chromiumtest.Start(t))
			require.NoError(t, err)
			ctx, cancel, err := browser.Tab(context.Background())
			require.NoError(t, err)
			defer cancel()
			ctx, cancel = context.WithTimeout(ctx, 35*time.Second)
			defer cancel()
			width := int64(1280)
			if phone {
				width = 390
			}
			_, err = chromedp.Call(ctx, emulation.SetDeviceMetricsOverride, emulation.SetDeviceMetricsOverrideParams{Width: width, Height: 844, DeviceScaleFactor: 1, Mobile: phone})
			require.NoError(t, err)
			require.NoError(t, chromedp.Do(ctx, chromedp.Navigate(app.URL), chromedp.WaitVisible(chromedp.CSS(`[data-gallery-query]`))))
			check := func(script string) {
				t.Helper()
				ok, err := chromedp.Run(ctx, chromedp.Evaluate[bool](script))
				require.NoError(t, err)
				require.True(t, ok, script)
			}
			poll := func(script string) {
				t.Helper()
				err := chromedp.Do(ctx, chromedp.Poll[chromedp.Void]("Boolean("+script+")", chromedp.WithPollingTimeout(6*time.Second)))
				if err != nil {
					state, _ := chromedp.Run(ctx, chromedp.Evaluate[string](`JSON.stringify({url:location.href,query:document.querySelector('[data-gallery-query]').value,active:document.activeElement.outerHTML,dialog:document.querySelector('.gallery-filter-sheet')?.open,cards:document.querySelectorAll('.card').length,html:document.querySelector('[data-gallery-results]').innerHTML,params:document.querySelector('[data-gallery-search]').innerHTML})`))
					t.Log(state)
				}
				require.NoError(t, err, script)
			}
			scope := ".gallery-rail-filters"
			if phone {
				scope = "#library-gallery-sheet-filters"
				require.NoError(t, chromedp.Do(ctx, chromedp.Click(chromedp.CSS(`.gallery-filter-trigger`)), chromedp.WaitVisible(chromedp.CSS(`dialog[open]`))))
				check(`document.querySelector('dialog[open]').contains(document.activeElement)`)
				require.NoError(t, chromedp.Do(ctx, chromedp.KeyEvent(kb.Escape)))
				check(`!document.querySelector('.gallery-filter-sheet').open && document.activeElement.matches('.gallery-filter-trigger')`)
				require.NoError(t, chromedp.Do(ctx, chromedp.Click(chromedp.CSS(`.gallery-filter-trigger`)), chromedp.MouseClickXY(4, 4)))
				poll(`!document.querySelector('.gallery-filter-sheet').open`)
				check(`document.activeElement.matches('.gallery-filter-trigger')`)
				require.NoError(t, chromedp.Do(ctx, chromedp.Click(chromedp.CSS(`.gallery-filter-trigger`))))
				check(`getComputedStyle(document.querySelector('.gallery-rail')).display === 'none' && document.documentElement.scrollWidth <= innerWidth`)
			} else {
				check(`document.querySelector('.gallery-rail').getBoundingClientRect().width === 208 && getComputedStyle(document.querySelector('.gallery-filter-trigger')).display === 'none'`)
			}
			require.NoError(t, chromedp.Do(ctx, chromedp.Click(chromedp.CSS(scope+` a[href="/?tag=Work"]`))))
			poll(`location.search === '?tag=Work' && document.querySelectorAll('.card').length === 1 && document.querySelector('.gallery-filter-chips')`)
			check(`document.querySelector('[data-gallery-search] input[name=tag]').value === 'Work' && document.querySelectorAll('a[data-active=true]').length === 2`)
			require.NoError(t, chromedp.Do(ctx, chromedp.Click(chromedp.CSS(scope+` a[href="/?collection=collection-1-0&tag=Work"]`))))
			poll(`new URL(location.href).searchParams.get('collection') === 'collection-1-0' && document.querySelector('.gallery-filter-clear')`)
			if phone {
				require.NoError(t, chromedp.Do(ctx, chromedp.KeyEvent(kb.Escape)))
				poll(`!document.querySelector('.gallery-filter-sheet').open && document.activeElement.matches('.gallery-filter-trigger')`)
			}
			require.NoError(t, chromedp.Do(ctx, chromedp.Focus(chromedp.CSS(`[data-gallery-query]`)), chromedp.SendKeys(chromedp.CSS(`[data-gallery-query]`), "Artifact")))
			poll(`new URL(location.href).searchParams.get('q') === 'Artifact'`)
			check(`new URL(location.href).searchParams.get('tag') === 'Work' && new URL(location.href).searchParams.get('collection') === 'collection-1-0' && document.querySelectorAll('.card').length === 1`)
			require.NoError(t, chromedp.Do(ctx, chromedp.Click(chromedp.CSS(`.gallery-filter-clear`))))
			poll(`location.search === '?q=Artifact' && !document.querySelector('.gallery-filter-chips') && document.querySelectorAll('.card').length === 2`)
			check(`document.querySelectorAll('[data-gallery-search] input[type=hidden]').length === 0 && document.querySelectorAll('a[data-active=true]').length === 0`)
			empty := httptest.NewServer(newTestRouter(t))
			defer empty.Close()
			require.NoError(t, chromedp.Do(ctx, chromedp.Navigate(empty.URL), chromedp.WaitVisible(chromedp.CSS("main"))))
			check(`!document.querySelector('.gallery-rail, .gallery-filter-trigger, .gallery-filter-sheet') && document.documentElement.scrollWidth <= innerWidth`)

		})
	}
}
