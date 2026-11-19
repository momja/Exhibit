package api

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/momja/Exhibit/internal/chromium"
	"github.com/momja/Exhibit/internal/chromium/chromiumtest"
	"github.com/stretchr/testify/require"
)

// Phone galleries hide the header and space tabs while scrolling down and
// bring them back on the way up (Exh-se4n). The direction rules themselves are
// scroll-chrome.test.mjs; this covers what only a browser shows: where the bars
// end up, focus, a space switch replacing the tabs, and desktop ignoring it.
func TestGalleryPhoneChromeBrowser(t *testing.T) {
	browser, err := chromium.New(chromiumtest.Start(t))
	require.NoError(t, err)
	ctx, cancel, err := browser.Tab(context.Background())
	require.NoError(t, err)
	defer cancel()
	ctx, cancel = context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	in := newGrantedInstance(t)
	seedSharedGallery(t, in, 30)
	_, err = in.ro.cfg.Store.CreateGrant(ctx, in.ownerTwo, in.artifactTwo, in.ownerOne)
	require.NoError(t, err)
	app := httptest.NewServer(in.ro)
	defer app.Close()
	in.ro.cfg.AppOrigin = app.URL
	_, err = chromedp.Call(ctx, network.SetCookie, network.SetCookieParams{Name: in.cookieTwo.Name, Value: in.cookieTwo.Value, URL: app.URL, Path: "/"})
	require.NoError(t, err)

	// Without the slide the bars settle in the frame the decision is made,
	// so the positions below need no waiting on a transition.
	_, err = chromedp.Call(ctx, emulation.SetDeviceMetricsOverride, emulation.SetDeviceMetricsOverrideParams{Width: 390, Height: 844, DeviceScaleFactor: 1, Mobile: true})
	require.NoError(t, err)
	_, err = chromedp.Call(ctx, emulation.SetEmulatedMedia, emulation.SetEmulatedMediaParams{Features: []*emulation.MediaFeature{{Name: "prefers-reduced-motion", Value: "reduce"}}})
	require.NoError(t, err)

	const (
		shown = `(() => { const h = document.querySelector('[data-app-header]').getBoundingClientRect(), r = document.querySelector('.gallery-rail').getBoundingClientRect();
			return Math.abs(h.top) < 1 && Math.abs(r.bottom - innerHeight) < 1; })()`
		hidden = `(() => { const h = document.querySelector('[data-app-header]').getBoundingClientRect(), r = document.querySelector('.gallery-rail').getBoundingClientRect();
			return h.bottom <= 0 && r.top >= innerHeight; })()`
	)
	poll := func(script, message string) {
		t.Helper()
		pollCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		require.NoError(t, chromedp.Do(pollCtx, chromedp.Poll[chromedp.Void](script)), message)
	}
	// GALLERY_CHROME_SCREENSHOT_DIR keeps the docs' screenshots current.
	capture := func(name string) {
		t.Helper()
		if dir := os.Getenv("GALLERY_CHROME_SCREENSHOT_DIR"); dir != "" {
			require.NoError(t, os.MkdirAll(dir, 0755))
			shot, err := chromedp.Call(ctx, page.CaptureScreenshot, page.CaptureScreenshotParams{})
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(dir, name+".png"), shot.Data, 0644))
		}
	}
	eval := func(script string) {
		t.Helper()
		require.NoError(t, chromedp.Do(ctx, chromedp.Evaluate[chromedp.Void](script)))
	}

	require.NoError(t, chromedp.Do(ctx, chromedp.Navigate(app.URL+"/shared"), chromedp.WaitVisible(chromedp.CSS(`.gallery-more button`))))
	poll(shown, "both bars start on screen")

	eval(`window.scrollTo(0, 600)`)
	poll(hidden, "scrolling down moves the header above the viewport and the tabs below it")
	capture("01-scrolling-down-390")
	eval(`window.scrollBy(0, -40)`)
	poll(shown, "scrolling back up returns both bars to their places")
	capture("02-scrolling-up-390")

	eval(`window.scrollTo(0, 1200)`)
	poll(hidden, "a second scroll down hides them again")
	eval(`window.scrollTo(0, 0)`)
	poll(shown, "the top of the page always shows them")

	// Keyboard focus entering a hidden bar brings it back where the reader
	// is and pins it. The mobile search field lives on its own page, so use
	// the header's Add artifact link to exercise the shared bar behavior.
	eval(`window.scrollTo(0, 600)`)
	poll(hidden, "hidden before a header control takes focus")
	eval(`document.querySelector('[data-app-header] a[href="/new"]').focus()`)
	poll(shown, "focusing a header control reveals the bars")
	poll(`window.scrollY === 600`, "focusing a hidden field keeps the reader's place")
	eval(`window.scrollBy(0, 400)`)
	poll(`window.scrollY === 1000`, "the page scrolled further")
	require.NoError(t, chromedp.Do(ctx, chromedp.Evaluate[chromedp.Void](`new Promise(done => requestAnimationFrame(() => requestAnimationFrame(done)))`, chromedp.EvalAwaitPromise)))
	poll(shown, "the bars stay while a header control has keyboard focus")
	eval(`document.activeElement.blur(); window.scrollBy(0, 100)`)
	poll(hidden, "once focus leaves, scrolling down hides them again")

	// A space switch swaps in new tabs, which must follow the same state.
	require.NoError(t, chromedp.Do(ctx, chromedp.Navigate(app.URL+"/shared-by-me"), chromedp.WaitVisible(chromedp.CSS(`.gallery-spaces`))))
	poll(shown, "a short gallery shows its bars")
	eval(`window.oldRail = document.querySelector('.gallery-rail'); document.querySelector('.gallery-spaces a[href="/shared"]').click()`)
	poll(`document.querySelector('.gallery-rail') !== window.oldRail && document.querySelector('.gallery-more button') !== null`, "the switch replaced the tabs")
	eval(`window.scrollTo(0, 600)`)
	poll(hidden, "the swapped-in tabs hide with the header")

	// Desktop keeps its sticky header and rail whatever the attribute says.
	_, err = chromedp.Call(ctx, emulation.SetDeviceMetricsOverride, emulation.SetDeviceMetricsOverrideParams{Width: 1280, Height: 800, DeviceScaleFactor: 1})
	require.NoError(t, err)
	poll(`window.scrollY > 0 && Math.abs(document.querySelector('[data-app-header]').getBoundingClientRect().top) < 1 && getComputedStyle(document.querySelector('.gallery-rail')).transform === 'none'`, "desktop ignores the phone behaviour")
}
