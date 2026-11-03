package api

import (
	"context"
	"fmt"
	"net/http"
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

// The tabs must reserve their rendered height, not an estimate that stops
// working when a label wraps, text grows, or the home indicator takes space.
func TestGalleryMobileTabsBrowser(t *testing.T) {
	browser, err := chromium.New(chromiumtest.Start(t))
	require.NoError(t, err)
	ctx, cancel, err := browser.Tab(context.Background())
	require.NoError(t, err)
	defer cancel()
	ctx, cancel = context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()

	in := newGrantedInstance(t)
	seedSharedGallery(t, in, 30)
	_, err = in.ro.cfg.Store.CreateGrant(ctx, in.ownerTwo, in.artifactTwo, in.ownerOne)
	require.NoError(t, err)
	// Revealed paging starts automatically when the sentinel scrolls into
	// view. Hold its response while measuring the pending control's clearance.
	pageResponses := make(chan struct{}, 1)
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/partials/gallery-grid/shared" && r.URL.Query().Get("offset") != "" {
			select {
			case <-pageResponses:
			case <-r.Context().Done():
				return
			}
		}
		in.ro.ServeHTTP(w, r)
	}))
	defer app.Close()
	defer close(pageResponses)
	in.ro.cfg.AppOrigin = app.URL
	_, err = chromedp.Call(ctx, network.SetCookie, network.SetCookieParams{Name: in.cookieTwo.Name, Value: in.cookieTwo.Value, URL: app.URL, Path: "/"})
	require.NoError(t, err)

	for _, width := range []int64{320, 375, 390, 430} {
		for _, scheme := range []string{"light", "dark"} {
			t.Run(fmt.Sprintf("%d-%s", width, scheme), func(t *testing.T) {
				_, err := chromedp.Call(ctx, emulation.SetDeviceMetricsOverride, emulation.SetDeviceMetricsOverrideParams{Width: width, Height: 844, DeviceScaleFactor: 1, Mobile: true})
				require.NoError(t, err)
				_, err = chromedp.Call(ctx, emulation.SetEmulatedMedia, emulation.SetEmulatedMediaParams{Features: []*emulation.MediaFeature{{Name: "prefers-color-scheme", Value: scheme}}})
				require.NoError(t, err)
				inset := int64(0)
				_, err = chromedp.Call(ctx, emulation.SetSafeAreaInsetsOverride, emulation.SetSafeAreaInsetsOverrideParams{Insets: &emulation.SafeAreaInsets{Bottom: &inset}})
				require.NoError(t, err)
				require.NoError(t, chromedp.Do(ctx, chromedp.Navigate(app.URL+"/shared"), chromedp.WaitVisible(chromedp.CSS(`.gallery-more button`)), chromedp.Evaluate[chromedp.Void](`document.fonts.ready`, chromedp.EvalAwaitPromise)))

				check := func(script, message string) {
					t.Helper()
					ok, err := chromedp.Run(ctx, chromedp.Evaluate[bool](script))
					require.NoError(t, err)
					require.True(t, ok, message)
				}
				capture := func(stage string) {
					t.Helper()
					if dir := os.Getenv("GALLERY_TABS_SCREENSHOT_DIR"); dir != "" {
						require.NoError(t, os.MkdirAll(dir, 0755))
						shot, err := chromedp.Call(ctx, page.CaptureScreenshot, page.CaptureScreenshotParams{})
						require.NoError(t, err)
						require.NoError(t, os.WriteFile(filepath.Join(dir, fmt.Sprintf("%d-%s-%s.png", width, scheme, stage)), shot.Data, 0644))
					}
				}
				// The reserved clearance follows the bar a frame after it resizes.
				settled := func() {
					t.Helper()
					require.NoError(t, chromedp.Do(ctx, chromedp.Poll[chromedp.Void](`Math.abs(parseFloat(getComputedStyle(document.querySelector('main')).paddingBottom) - 12 - document.querySelector('.gallery-rail').getBoundingClientRect().height) < 1`)))
				}
				settled()
				check(`(() => { const r = document.querySelector('.gallery-rail').getBoundingClientRect(); return r.height >= 52 && r.height <= (innerWidth === 320 ? 72 : 56) && Math.abs(r.bottom - innerHeight) < 1; })()`, "normal tabs stay compact, allow full labels to wrap at 320px, and sit at the viewport bottom")
				check(`Array.from(document.querySelectorAll('.gallery-spaces a')).every(a => a.getBoundingClientRect().height >= 44 && a.getBoundingClientRect().width >= 44) && document.documentElement.scrollWidth <= innerWidth`, "targets stay at least 44×44px without horizontal overflow")
				capture("normal")

				// Enlarge the actual full labels rendered by the server.
				require.NoError(t, chromedp.Do(ctx, chromedp.Evaluate[chromedp.Void](`document.documentElement.style.fontSize = '200%';`)))
				inset = 34
				_, err = chromedp.Call(ctx, emulation.SetSafeAreaInsetsOverride, emulation.SetSafeAreaInsetsOverrideParams{Insets: &emulation.SafeAreaInsets{Bottom: &inset}})
				require.NoError(t, err)
				check(`parseFloat(getComputedStyle(document.querySelector('.gallery-spaces a')).fontSize) === 24 && document.querySelector('.gallery-rail').getBoundingClientRect().height > 90`, "200% text and full labels grow the bar beyond its normal height plus inset")
				check(`document.querySelectorAll('.space-label').length === 3 && Array.from(document.querySelectorAll('.space-label')).every(label => { const r = label.getBoundingClientRect(), a = label.closest('a').getBoundingClientRect(); return label.scrollWidth <= label.clientWidth && r.left >= a.left && r.right <= a.right && r.top >= a.top && r.bottom <= a.bottom; }) && document.documentElement.scrollWidth <= innerWidth`, "enlarged full labels wrap inside their targets without clipping or horizontal overflow")
				check(`getComputedStyle(document.querySelector('.gallery-rail')).paddingBottom === '38px' && Array.from(document.querySelectorAll('.gallery-spaces a')).every(a => a.getBoundingClientRect().bottom <= innerHeight - 34)`, "targets stay above the simulated home indicator")
				settled()
				require.NoError(t, chromedp.Do(ctx, chromedp.Evaluate[chromedp.Void](`window.scrollTo(0, document.documentElement.scrollHeight / 2)`)))
				check(`Math.abs(document.querySelector('.gallery-rail').getBoundingClientRect().bottom - innerHeight) < 1`, "the growing rail stays on screen during document scrolling")
				require.NoError(t, chromedp.Do(ctx, chromedp.Evaluate[chromedp.Void](`window.scrollTo(0, document.documentElement.scrollHeight)`)))
				// Wait for automatic paging's pending layout before measuring.
				require.NoError(t, chromedp.Do(ctx,
					chromedp.Poll[chromedp.Void](`document.querySelector('.gallery-more button').classList.contains('htmx-request')`),
					chromedp.Evaluate[chromedp.Void](`window.scrollTo(0, document.documentElement.scrollHeight)`)))
				check(`document.querySelector('.gallery-more').getBoundingClientRect().bottom + 11 <= document.querySelector('.gallery-rail').getBoundingClientRect().top`, "Load more clears the actual enlarged bar and its inset with a content gap")
				capture("large-labels-pagination")
				select {
				case pageResponses <- struct{}{}:
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}
				require.NoError(t, chromedp.Do(ctx, chromedp.Poll[chromedp.Void](`document.querySelectorAll('.card').length > 24 && !document.querySelector('.gallery-more')`), chromedp.Evaluate[chromedp.Void](`window.scrollTo(0, document.documentElement.scrollHeight)`)))
				check(`Array.from(document.querySelectorAll('.card')).at(-1).getBoundingClientRect().bottom + 11 <= document.querySelector('.gallery-rail').getBoundingClientRect().top`, "the last appended card also clears the enlarged tabs")

				// Returning to normal size must release the extra space too.
				require.NoError(t, chromedp.Do(ctx, chromedp.Evaluate[chromedp.Void](`document.documentElement.style.fontSize = ''`)))
				settled()
				require.NoError(t, chromedp.Do(ctx, chromedp.Evaluate[chromedp.Void](`window.scrollTo(0, document.documentElement.scrollHeight)`)))
				check(`(() => { const gap = document.querySelector('.gallery-rail').getBoundingClientRect().top - Array.from(document.querySelectorAll('.card')).at(-1).getBoundingClientRect().bottom; return gap >= 11 && gap <= 24; })()`, "content clearance follows shrinking tabs")
				// One outgoing artifact exercises a page shorter than the viewport.
				require.NoError(t, chromedp.Do(ctx, chromedp.Navigate(app.URL+"/shared-by-me"), chromedp.WaitVisible(chromedp.CSS(`.gallery-spaces`))))
				check(`Math.abs(document.querySelector('.gallery-rail').getBoundingClientRect().bottom - innerHeight) < 1 && document.documentElement.scrollHeight <= innerHeight + 1`, "short galleries keep tabs at the bottom without a spurious scroll range")
				// iOS rubber-banding shows the screen past the end of the content.
				// Ending the content above the viewport bottom stands in for that:
				// a sticky bar is held inside the layout and rides up with it.
				require.NoError(t, chromedp.Do(ctx, chromedp.Evaluate[chromedp.Void](`document.querySelector('.gallery-layout').style.minHeight = '0'`)))
				check(`document.querySelector('main').getBoundingClientRect().bottom < innerHeight - 100 && Math.abs(document.querySelector('.gallery-rail').getBoundingClientRect().bottom - innerHeight) < 1`, "tabs stay at the viewport bottom when the screen extends past the content")
			})
		}
	}
}
