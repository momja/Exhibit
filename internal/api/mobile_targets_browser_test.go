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
	"github.com/chromedp/chromedp/kb"
	"github.com/momja/Exhibit/internal/chromium"
	"github.com/momja/Exhibit/internal/chromium/chromiumtest"
	"github.com/stretchr/testify/require"
)

// Check the real clickable boxes, not just their glyphs. A click near an edge
// must open the intended control rather than the card's delegated navigation.
func TestMobileTargetsBrowser(t *testing.T) {
	in := newProfileInstance(t, "targets-admin", "targets@example.test")
	seedGalleryFilters(t, in.ro, in.user.ID)
	id := createArtifact(t, in.ro, map[string]any{"title": "A long title that must leave room for independent card actions", "body": "<!doctype html><h1>Targets</h1>"})
	require.NoError(t, in.st.UpdateArtifact(context.Background(), in.user.ID, id, map[string]any{"downloads_approved": true, "clipboard_approved": true, "camera_approved": true, "microphone_approved": true, "geolocation_approved": true, "links_approved": true}))
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { r.AddCookie(in.cookie); in.ro.ServeHTTP(w, r) }))
	defer app.Close()
	in.ro.cfg.AppOrigin = app.URL
	browser, err := chromium.New(chromiumtest.Start(t))
	require.NoError(t, err)
	ctx, cancel, err := browser.Tab(context.Background())
	require.NoError(t, err)
	defer cancel()
	ctx, cancel = context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	for _, width := range []int64{320, 375, 390, 430} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
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
			_, err := chromedp.Call(ctx, emulation.SetDeviceMetricsOverride, emulation.SetDeviceMetricsOverrideParams{Width: width, Height: 844, DeviceScaleFactor: 1})
			require.NoError(t, err)
			run(chromedp.Navigate(app.URL+"/?q=long"), chromedp.Poll[chromedp.Void](`document.querySelector('.card-actions-trigger') && !document.querySelector('[data-gallery-clear]').hidden`))
			check(`(() => {
    const header = [...document.querySelectorAll('.app-header a,.app-header button,.app-header input:not([type=hidden])')];
    const card = [...document.querySelectorAll('.card-title,[data-capability-trigger],.card-actions-trigger')];
    const visible = list => list.filter(e => e.getBoundingClientRect().width > 0);
    const apart = list => visible(list).every((e,i,all) => {
     const r = e.getBoundingClientRect();
     if (r.width < 44 || r.height < 44) return false;
     return all.slice(i+1).every(other => {const s=other.getBoundingClientRect();return r.right<=s.left || s.right<=r.left || r.bottom<=s.top || s.bottom<=r.top});
    });
    return apart(header) && apart(card) && document.documentElement.scrollWidth <= innerWidth && !document.querySelector('header [data-gallery-query]').getClientRects().length;
   })()`)
			for _, scheme := range []string{"light", "dark"} {
				_, err := chromedp.Call(ctx, emulation.SetEmulatedMedia, emulation.SetEmulatedMediaParams{Features: []*emulation.MediaFeature{{Name: "prefers-color-scheme", Value: scheme}}})
				require.NoError(t, err)
				modalBrowser{t, ctx}.capture("Exh-1og2/gallery-"+scheme, width)
			}
			// Use a point inside the new padding, away from the original tiny glyph.
			for _, target := range []struct{ selector, open string }{{".card-actions-trigger", ".card-actions.is-open"}, {"[data-capability-trigger]", ".capability-wrap.is-open"}} {
				point, err := chromedp.Run(ctx, chromedp.Evaluate[struct{ X, Y float64 }](fmt.Sprintf(`(() => {const r=document.querySelector(%q).getBoundingClientRect();return {X:r.left+2,Y:r.top+r.height/2}})()`, target.selector)))
				require.NoError(t, err)
				run(chromedp.MouseClickXY(point.X, point.Y), chromedp.Poll[chromedp.Void](fmt.Sprintf(`document.querySelector(%q)`, target.open), chromedp.WithPollingTimeout(5*time.Second)))
				check(`location.pathname === '/'`)
				run(chromedp.KeyEvent(kb.Escape))
			}
			run(chromedp.Click(chromedp.CSS(`[data-gallery-clear]`)), chromedp.Poll[chromedp.Void](`document.querySelector('[data-gallery-query]').value === '' && document.querySelector('[data-gallery-clear]').hidden`))
			check(`document.activeElement.matches('[data-gallery-query]')`)
			// Typography may wrap the title or change its leading/padding. The
			// anchor's target floor must not depend on any of those choices.
			run(chromedp.Evaluate[chromedp.Void](`Object.assign(document.querySelector('.card-title').style,{fontSize:'16px',lineHeight:'1.4',paddingBlock:'6px'})`))
			check(`document.querySelector('.card-title').getBoundingClientRect().height >= 44`)
			run(chromedp.Evaluate[chromedp.Void](`document.querySelector('.card-title').style.fontSize='25px'; document.querySelector('[data-gallery-query]').style.fontSize='32px'`))
			check(`document.documentElement.scrollWidth <= innerWidth`)
			modalBrowser{t, ctx}.capture("Exh-1og2/gallery-large-text", width)
			// Text-only enlargement, including a long editor return label, must not
			// squeeze its native anchor below the target size or overlap Save.
			run(chromedp.Navigate(app.URL+"/artifacts/"+id+"/edit?return_to=/artifacts/"+id), chromedp.Evaluate[chromedp.Void](`document.querySelector('.app-header-back-label').style.fontSize='28px'`))
			check(`(() => {const back=document.querySelector('.app-header-back').getBoundingClientRect(); const actions=document.querySelector('.app-header-actions').getBoundingClientRect(); return back.width>=44 && back.height>=44 && back.right<=actions.left && document.documentElement.scrollWidth<=innerWidth})()`)
			modalBrowser{t, ctx}.capture("Exh-1og2/editor-large-text", width)
			topInset := int64(24)
			_, err = chromedp.Call(ctx, emulation.SetSafeAreaInsetsOverride, emulation.SetSafeAreaInsetsOverrideParams{Insets: &emulation.SafeAreaInsets{Top: &topInset}})
			require.NoError(t, err)
			run(chromedp.Evaluate[chromedp.Void](`window.scrollTo(0,document.documentElement.scrollHeight)`))
			check(`document.querySelector('.app-header').getBoundingClientRect().height === 80 && document.querySelector('.app-header-back').getBoundingClientRect().top >= 24`)
			_, err = chromedp.Call(ctx, emulation.SetSafeAreaInsetsOverride, emulation.SetSafeAreaInsetsOverrideParams{Insets: &emulation.SafeAreaInsets{}})
			require.NoError(t, err)
		})
	}
	_, err = chromedp.Call(ctx, emulation.SetDeviceMetricsOverride, emulation.SetDeviceMetricsOverrideParams{Width: 1280, Height: 900, DeviceScaleFactor: 1})
	require.NoError(t, err)
	require.NoError(t, chromedp.Do(ctx, chromedp.Navigate(app.URL)))
	for _, scheme := range []string{"light", "dark"} {
		_, err := chromedp.Call(ctx, emulation.SetEmulatedMedia, emulation.SetEmulatedMediaParams{Features: []*emulation.MediaFeature{{Name: "prefers-color-scheme", Value: scheme}}})
		require.NoError(t, err)
		modalBrowser{t, ctx}.capture("Exh-1og2/gallery-"+scheme, 1280)
	}
}
