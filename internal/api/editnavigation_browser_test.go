package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/chromedp"
	"github.com/momja/Exhibit/internal/chromium"
	"github.com/momja/Exhibit/internal/chromium/chromiumtest"
	"github.com/stretchr/testify/require"
)

func TestEditNavigationBrowser(t *testing.T) {
	for _, width := range []int64{1280, 390} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			ro := newTestRouter(t)
			id := createArtifact(t, ro, map[string]any{"title": "Navigation tool", "body": "<!doctype html><h1>My tool</h1>"})
			for i := range 28 {
				createArtifact(t, ro, map[string]any{"title": fmt.Sprintf("Navigation sample %02d", i), "body": "<h1>Sample</h1>"})
			}
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
			ctx, cancel = context.WithTimeout(ctx, 90*time.Second)
			defer cancel()
			_, err = chromedp.Call(ctx, emulation.SetDeviceMetricsOverride, emulation.SetDeviceMetricsOverrideParams{Width: width, Height: 900, DeviceScaleFactor: 1})
			require.NoError(t, err)
			eval := func(js string) {
				t.Helper()
				require.NoError(t, chromedp.Do(ctx, chromedp.Evaluate[chromedp.Void](js)))
			}
			navigate := func(js string) {
				t.Helper()
				_, err := chromedp.RunResponse(ctx, chromedp.Evaluate[chromedp.Void](js))
				require.NoError(t, err)
			}
			wait := func(js string) {
				t.Helper()
				require.NoError(t, chromedp.Do(ctx, chromedp.Poll[chromedp.Void](js)))
			}
			check := func(js, message string) {
				t.Helper()
				ok, err := chromedp.Run(ctx, chromedp.Evaluate[bool](js))
				require.NoError(t, err)
				require.True(t, ok, message)
			}
			require.NoError(t, chromedp.Do(ctx, chromedp.Navigate(app.URL+"/?q=Navigation")))
			wait(`document.querySelectorAll('.card').length >= 24`)
			eval(`window.scrollTo(0, document.documentElement.scrollHeight)`)
			wait(`document.querySelectorAll('.card').length === 29`)
			eval(`window.scrollTo(0, document.documentElement.scrollHeight); sessionStorage.setItem('navigation-test-scroll', String(scrollY));`)
			navigate(fmt.Sprintf(`document.querySelector('.card[data-href="/artifacts/%s"] a[role="menuitem"]').click()`, id))
			wait(`document.querySelector('[data-unsaved-changes-target="status"]') && document.querySelector('.cm-editor')`)
			check(`document.querySelector('.app-header-back').textContent.includes('My library') && new URLSearchParams(location.search).get('return_to').includes('visible=29')`, "library entry preserves the loaded view")
			check(`document.querySelector('.app-header-actions').getBoundingClientRect().bottom <= document.querySelector('header').getBoundingClientRect().bottom && document.querySelector('header').getBoundingClientRect().bottom <= document.querySelector('.edit-heading').getBoundingClientRect().top`, "header actions stay inside the bar and above the editor")
			eval(`document.getElementById('title').value='Navigation renamed'; document.getElementById('title').dispatchEvent(new Event('input',{bubbles:true})); document.getElementById('save-button').click();`)
			wait(`document.getElementById('status').textContent.includes('Saved') && document.getElementById('save-button').disabled === false`)
			check(`location.pathname.endsWith('/edit') && document.querySelector('[data-unsaved-changes-target="status"]').textContent === 'All changes saved'`, "saving remains in the editor")
			navigate(`document.querySelector('.app-header-back').click()`)
			wait(`document.querySelectorAll('.card').length === 29 && scrollY > 0`)
			check(`Math.abs(scrollY - Number(sessionStorage.getItem('navigation-test-scroll'))) < 20 && new URLSearchParams(location.search).get('q') === 'Navigation'`, "return restores results, search, and scroll")
			navigate(fmt.Sprintf(`document.querySelector('.card[data-href="/artifacts/%s"] .card-title').click()`, id))
			wait(`document.getElementById('pip-more')`)
			check(`document.querySelector('#pip a[aria-label="My library"] .ph-squares-four') && document.querySelector('#pip a[aria-label="My library"]').textContent.trim() === ''`, "Library is an icon-only destination")
			navigate(`document.querySelector('#pip-menu a[href$="/edit"]').click()`)
			wait(`document.querySelector('.cm-editor') && document.querySelector('.app-header-back').textContent.includes('Navigation renamed')`)
			check(`new URLSearchParams(location.search).get('return_to') === location.pathname.replace('/edit','')`, "artifact return is recorded explicitly")
			navigate(`location.reload()`)
			wait(`document.querySelector('.cm-editor') && document.querySelector('.app-header-back').textContent.includes('Navigation renamed')`)
			require.NoError(t, chromedp.Do(ctx, chromedp.SendKeys(chromedp.CSS(`#source-panel .cm-content`), " ")))
			wait(`document.querySelector('[data-unsaved-changes-target="status"]').textContent === 'Unsaved changes'`)
			eval(`document.getElementById('save-button').click()`)
			wait(`document.querySelectorAll('#versions-rows .version-row').length === 2 && document.querySelector('[data-unsaved-changes-target="status"]').textContent === 'All changes saved'`)
			eval(`document.getElementById('state-panel').open=true`)
			wait(`document.getElementById('state-summary-text').textContent.includes('empty')`)
			eval(`document.getElementById('state-add-key').value='navigation-note'; document.getElementById('state-add-btn').click(); document.querySelector('.app-header-back').click()`)
			wait(`document.querySelector('.editor-leave-dialog').open`)
			navigate(`document.querySelector('[data-action="unsaved-changes#saveAndLeave"]').click()`)
			wait(`document.getElementById('pip-more')`)
			navigate(`document.querySelector('#pip-menu a[href$="/edit"]').click()`)
			wait(`document.querySelector('.cm-editor')`)
			eval(`document.getElementById('title').value='Discard this'; document.getElementById('title').dispatchEvent(new Event('input',{bubbles:true})); document.getElementById('geo-select').value='true'; document.getElementById('geo-select').dispatchEvent(new Event('change',{bubbles:true})); document.querySelector('.app-header-back').click()`)
			wait(`document.querySelector('.editor-leave-dialog').open`)
			check(`document.activeElement.hasAttribute('data-keep-editing') && document.documentElement.scrollWidth <= innerWidth`, "dialog focus and phone layout")
			eval(`document.querySelector('[data-keep-editing]').click()`)
			check(`document.getElementById('title').value === 'Discard this' && !document.querySelector('.editor-leave-dialog').open`, "keep editing preserves drafts")
			navigate(`document.querySelector('.app-header-back').click(); document.querySelector('[data-action="unsaved-changes#discard"]').click()`)
			wait(`document.getElementById('pip-more')`)
			navigate(`document.querySelector('#pip-menu a[href$="/edit"]').click()`)
			wait(`document.querySelector('.cm-editor')`)
			check(`document.getElementById('title').value === 'Navigation renamed' && document.getElementById('geo-select').value === 'false'`, "discard did not save title or security edits")
			navigate(`document.getElementById('title').value='Navigation saved on exit'; document.getElementById('title').dispatchEvent(new Event('input',{bubbles:true})); document.querySelector('.app-header-back').click(); document.querySelector('[data-action="unsaved-changes#saveAndLeave"]').click()`)
			wait(`document.getElementById('pip-info-title')?.textContent === 'Navigation saved on exit'`)
			navigate(`document.querySelector('#pip a[aria-label="My library"]').click()`)
			wait(`document.querySelector('[data-gallery]')`)
			check(`location.pathname === '/' && location.search === ''`, "Library does not depend on history")
			eval(`history.back()`)
			require.NoError(t, chromedp.Do(ctx, chromedp.WaitVisible(chromedp.CSS(`#pip-more`))))
			eval(`history.back()`)
			require.NoError(t, chromedp.Do(ctx, chromedp.WaitVisible(chromedp.CSS(`.app-header-back`))))
			eval(`document.getElementById('title').value='Another draft after Back'; document.getElementById('title').dispatchEvent(new Event('input',{bubbles:true}));`)
			check(`(() => { const event = new Event('beforeunload',{cancelable:true}); window.dispatchEvent(event); return event.defaultPrevented; })()`, "native leave protection is restored after browser Back")
		})
	}
}

func TestEditNavigationPermissionSavesBrowser(t *testing.T) {
	for _, width := range []int64{1280, 390} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			ro := newTestRouter(t)
			id := createArtifact(t, ro, map[string]any{"title": "Permissions tool", "body": `<script src="https://blocked.example.test/lib.js"></script>`})
			block := func() {
				t.Helper()
				require.Equal(t, http.StatusOK, doJSON(t, ro, "POST", "/api/artifacts/"+id+"/origins", map[string]any{"origin": "https://blocked.example.test", "decision": "block"}).Code)
			}
			block()
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
			_, err = chromedp.Call(ctx, emulation.SetDeviceMetricsOverride, emulation.SetDeviceMetricsOverrideParams{Width: width, Height: 900, DeviceScaleFactor: 1})
			require.NoError(t, err)
			eval := func(js string) {
				t.Helper()
				require.NoError(t, chromedp.Do(ctx, chromedp.Evaluate[chromedp.Void](js)))
			}
			wait := func(js string) {
				t.Helper()
				if err := chromedp.Do(ctx, chromedp.Poll[chromedp.Void](js)); err != nil {
					state, inspectErr := chromedp.Run(ctx, chromedp.Evaluate[string](`JSON.stringify({origins:document.getElementById('origin-lists')?.innerHTML, status:document.querySelector('[data-unsaved-changes-target="status"]')?.textContent, allowlist, blocked, unapproved, footprint:INITIAL_SOURCE_FOOTPRINT})`))
					t.Logf("editor state: %s (inspection error: %v)", state, inspectErr)
					require.NoError(t, err)
				}
			}
			save := func() {
				t.Helper()
				eval(`document.getElementById('save-button').click()`)
				wait(`document.getElementById('save-button').disabled === false && document.querySelector('[data-unsaved-changes-target="status"]').textContent === 'All changes saved'`)
			}
			require.NoError(t, chromedp.Do(ctx, chromedp.Navigate(app.URL+"/artifacts/"+id+"/edit#security-panel")))
			wait(`document.querySelector('#blocked-rows [data-action="forget"]') && document.querySelector('.cm-editor') && document.querySelector('[data-unsaved-changes-target="status"]').textContent === 'All changes saved'`)
			// A group absent at initial load must appear through an htmx swap.
			eval(`document.querySelector('#blocked-rows [data-action="forget"]').click()`)
			wait(`document.querySelector('#unapproved-rows [data-origin="https://blocked.example.test"]') && !document.getElementById('blocked-rows') && document.querySelector('[data-unsaved-changes-target="status"]').textContent === 'Unsaved changes'`)
			save()
			decisions, err := ro.cfg.Store.ListOriginDecisions(ctx, 1, id)
			require.NoError(t, err)
			require.Empty(t, decisions)
			eval(`document.querySelector('#unapproved-rows [data-action="allow"]').click()`)
			wait(`document.querySelector('#allowlist-rows [data-origin="https://blocked.example.test"]') && !document.getElementById('unapproved-rows')`)
			save()
			eval(`document.querySelector('#allowlist-rows [data-action="remove"]').click()`)
			wait(`document.querySelector('#unapproved-rows [data-origin="https://blocked.example.test"]') && document.querySelector('[data-unsaved-changes-target="status"]').textContent === 'Unsaved changes'`)
			save()
			eval(`for (const origin of ['https://first.example.test','https://last.example.test']) { document.getElementById('al-add-input').value=origin; document.getElementById('al-add-btn').click(); }`)
			wait(`document.querySelectorAll('#allowlist-rows .allowlist-row').length === 2 && document.querySelector('[data-unsaved-changes-target="status"]').textContent === 'Unsaved changes'`)
			save()
			block()
			require.NoError(t, chromedp.Do(ctx, chromedp.Navigate(app.URL+"/artifacts/"+id+"/edit#security-panel")))
			wait(`document.querySelector('.cm-editor') && document.querySelector('#blocked-rows [data-action="forget"]')`)
			require.NoError(t, chromedp.Do(ctx, chromedp.SendKeys(chromedp.CSS(`#source-panel .cm-content`), `<script src="https://new.example.test/lib.js"></script>`)))
			eval(`document.getElementById('save-button').click()`)
			wait(`document.getElementById('scan-result').style.display === 'block' && document.getElementById('save-button').disabled === false`)
			eval(`document.querySelectorAll('.al-origin').forEach(box=>box.checked=true); window.approveOrigins()`)
			wait(`!document.getElementById('blocked-rows') && !document.getElementById('unapproved-rows') && document.querySelector('#allowlist-rows [data-origin="https://new.example.test"]') && document.querySelector('[data-unsaved-changes-target="status"]').textContent === 'All changes saved'`)
			// The viewer's independent grant makes the bootstrap value stale.
			require.Equal(t, http.StatusOK, doJSON(t, ro, "PATCH", "/api/artifacts/"+id, map[string]any{"geolocation_approved": true}).Code)
			eval(`for (const value of ['true','false']) { document.getElementById('geo-select').value=value; document.getElementById('geo-select').dispatchEvent(new Event('change',{bubbles:true})); }`)
			wait(`document.querySelector('[data-unsaved-changes-target="status"]').textContent === 'Unsaved changes'`)
			save()
			artifact, err := ro.cfg.Store.GetArtifact(ctx, 1, id)
			require.NoError(t, err)
			require.False(t, artifact.GeolocationApproved)
			// Whitespace in an absent widget must not trap Save and leave.
			eval(`document.getElementById('widget-src').value=' \n '; document.getElementById('widget-src').dispatchEvent(new Event('input',{bubbles:true})); document.querySelector('.app-header-actions a').click()`)
			wait(`document.querySelector('.editor-leave-dialog').open`)
			_, err = chromedp.RunResponse(ctx, chromedp.Evaluate[chromedp.Void](`document.querySelector('[data-action="unsaved-changes#saveAndLeave"]').click()`))
			require.NoError(t, err)
			wait(`document.getElementById('pip-more')`)
		})
	}
}
