package api

import (
	"context"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
	"github.com/momja/Exhibit/internal/chromium"
	"github.com/momja/Exhibit/internal/chromium/chromiumtest"
	"github.com/stretchr/testify/require"
)

func TestGallerySpacesBrowser(t *testing.T) {
	for _, viewport := range []struct {
		name          string
		width, height int64
		phone         bool
	}{{"phone", 390, 844, true}, {"desktop", 1280, 900, false}} {
		t.Run(viewport.name, func(t *testing.T) {
			browser, err := chromium.New(chromiumtest.Start(t))
			require.NoError(t, err)
			ctx, cancel, err := browser.Tab(context.Background())
			require.NoError(t, err)
			defer cancel()
			ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
			defer cancel()

			in := newGrantedInstance(t)
			seedSharedGallery(t, in, 18)
			_, err = in.ro.cfg.Store.CreateGrant(ctx, in.ownerTwo, in.artifactTwo, in.ownerOne)
			require.NoError(t, err)
			app := httptest.NewServer(in.ro)
			defer app.Close()
			in.ro.cfg.AppOrigin = app.URL
			_, err = chromedp.Call(ctx, network.SetCookie, network.SetCookieParams{Name: in.cookieTwo.Name, Value: in.cookieTwo.Value, URL: app.URL, Path: "/"})
			require.NoError(t, err)
			_, err = chromedp.Call(ctx, emulation.SetDeviceMetricsOverride, emulation.SetDeviceMetricsOverrideParams{Width: viewport.width, Height: viewport.height, DeviceScaleFactor: 1, Mobile: viewport.phone})
			require.NoError(t, err)
			if viewport.phone {
				bottomInset := int64(34)
				_, err = chromedp.Call(ctx, emulation.SetSafeAreaInsetsOverride, emulation.SetSafeAreaInsetsOverrideParams{Insets: &emulation.SafeAreaInsets{Bottom: &bottomInset}})
				require.NoError(t, err)
			}
			require.NoError(t, chromedp.Do(ctx, chromedp.Navigate(app.URL+"/shared"), chromedp.WaitVisible(chromedp.CSS(`.gallery-spaces`))))
			check := func(script, message string) {
				t.Helper()
				ok, err := chromedp.Run(ctx, chromedp.Evaluate[bool](script))
				require.NoError(t, err)
				require.True(t, ok, message)
			}
			check(`document.querySelectorAll('nav[aria-label="Library spaces"]').length === 1 && document.querySelectorAll('.gallery-spaces [aria-current="page"]').length === 1 && document.querySelector('.gallery-spaces [aria-current="page"]').getAttribute('href') === '/shared'`, "one nav and the incoming space is active")
			check(`Array.from(document.querySelectorAll('.gallery-spaces a')).every(a => a.getBoundingClientRect().width >= 44 && a.getBoundingClientRect().height >= 44) && document.documentElement.scrollWidth <= innerWidth`, "all targets are at least 44px and the page fits")
			check(fmt.Sprintf(`Array.from(document.querySelectorAll('.space-count')).every(el => (getComputedStyle(el).display === 'none') === %t)`, viewport.phone), "counts appear only in the rail")
			if viewport.phone {
				check(`getComputedStyle(document.querySelector('.gallery-rail')).paddingBottom === '38px' && getComputedStyle(document.querySelector('main')).paddingBottom === '114px'`, "the phone home-indicator inset is reserved in both the tabs and content")
				check(`getComputedStyle(document.querySelector('.gallery-rail')).position === 'fixed' && Math.abs(document.querySelector('.gallery-rail').getBoundingClientRect().bottom - innerHeight) < 1`, "phone tabs are fixed to the viewport bottom")
				require.NoError(t, chromedp.Do(ctx, chromedp.Evaluate[chromedp.Void](`window.scrollTo(0, document.documentElement.scrollHeight)`)))
				check(`Array.from(document.querySelectorAll('.card')).at(-1).getBoundingClientRect().bottom <= document.querySelector('.gallery-rail').getBoundingClientRect().top`, "the last card scrolls fully above the tabs")
			} else {
				check(`getComputedStyle(document.querySelector('.gallery-layout')).display === 'grid' && document.querySelector('.gallery-rail').getBoundingClientRect().width === 208 && document.querySelector('.gallery-rail').getBoundingClientRect().right <= document.querySelector('main').getBoundingClientRect().left`, "desktop rail sits beside the grid")
				check(`Array.from(document.querySelectorAll('.space-label-wide')).every(el => el.scrollWidth <= el.clientWidth)`, "desktop labels fit alongside two-digit counts")
			}

			// Ordinary Tab order and visible focus work in both layouts.
			require.NoError(t, chromedp.Do(ctx, chromedp.Focus(chromedp.CSS(`.gallery-spaces a[href="/"]`)), chromedp.KeyEvent(kb.Tab)))
			check(`document.activeElement.getAttribute('href') === '/shared' && document.activeElement.matches(':focus-visible') && getComputedStyle(document.activeElement).outlineStyle === 'solid'`, "Tab reaches the incoming space with a visible outline")
			require.NoError(t, chromedp.Do(ctx, chromedp.KeyEvent(kb.Tab)))
			check(`document.activeElement.getAttribute('href') === '/shared-by-me'`, "Tab reaches outgoing shares next")
			_, err = chromedp.RunResponse(ctx, chromedp.KeyEvent(kb.Enter))
			require.NoError(t, err)
			check(`location.pathname === '/shared-by-me' && document.querySelector('.gallery-spaces [aria-current="page"]').getAttribute('href') === '/shared-by-me'`, "keyboard activation navigates to outgoing shares")
			for _, path := range []string{"/", "/shared"} {
				// Wait for the outgoing snapshot to stop covering pointer targets,
				// and for the full new document to load after each navigation.
				require.NoError(t, chromedp.Do(ctx, chromedp.Poll[chromedp.Void](`document.getAnimations().length === 0`)))
				_, err = chromedp.RunResponse(ctx, chromedp.Click(chromedp.CSS(`.gallery-spaces a[href="`+path+`"]`)))
				require.NoError(t, err)
				check(`location.pathname === '`+path+`' && document.querySelector('.gallery-spaces [aria-current="page"]').getAttribute('href') === '`+path+`'`, "plain links navigate and update the active space")
			}
			_, err = chromedp.Call(ctx, emulation.SetEmulatedMedia, emulation.SetEmulatedMediaParams{Features: []*emulation.MediaFeature{{Name: "prefers-color-scheme", Value: "dark"}}})
			require.NoError(t, err)
			check(`matchMedia('(prefers-color-scheme: dark)').matches && getComputedStyle(document.querySelector('.gallery-rail')).backgroundColor === getComputedStyle(document.querySelector('header')).backgroundColor`, "the navigation follows the dark panel token")
		})
	}
}
