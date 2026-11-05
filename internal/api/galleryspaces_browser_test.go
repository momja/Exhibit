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
			check(`document.querySelectorAll('nav[aria-label="Gallery spaces"]').length === 1 && document.querySelectorAll('.gallery-spaces [aria-current="page"]').length === 1 && document.querySelector('.gallery-spaces [aria-current="page"]').getAttribute('href') === '/shared'`, "one nav and the incoming space is active")
			check(`Array.from(document.querySelectorAll('.gallery-spaces a')).filter(a => a.getClientRects().length).every(a => a.getBoundingClientRect().width >= 44 && a.getBoundingClientRect().height >= 44) && document.documentElement.scrollWidth <= innerWidth`, "all targets are at least 44px and the page fits")
			check(fmt.Sprintf(`Array.from(document.querySelectorAll('.space-count')).every(el => (getComputedStyle(el).display === 'none') === %t)`, viewport.phone), "counts appear only in the rail")
			if viewport.phone {
				check(`getComputedStyle(document.querySelector('.gallery-rail')).paddingBottom === '38px' && document.querySelector('.gallery-rail').getBoundingClientRect().height >= 100 && document.querySelector('.gallery-rail').getBoundingClientRect().height <= 108`, "the compact tabs include the phone home-indicator inset")
				check(`getComputedStyle(document.querySelector('.gallery-rail')).position === 'fixed' && Math.abs(document.querySelector('.gallery-rail').getBoundingClientRect().bottom - innerHeight) < 1`, "phone tabs are pinned to the viewport bottom")
				require.NoError(t, chromedp.Do(ctx, chromedp.Evaluate[chromedp.Void](`window.scrollTo(0, document.documentElement.scrollHeight)`)))
				check(`Array.from(document.querySelectorAll('.card')).at(-1).getBoundingClientRect().bottom <= document.querySelector('.gallery-rail').getBoundingClientRect().top`, "the last card scrolls fully above the tabs")
			} else {
				check(`getComputedStyle(document.querySelector('.gallery-layout')).display === 'grid' && document.querySelector('.gallery-rail').getBoundingClientRect().width === 208 && document.querySelector('.gallery-rail').getBoundingClientRect().right <= document.querySelector('main').getBoundingClientRect().left`, "desktop rail sits beside the grid")
				check(`Array.from(document.querySelectorAll('.space-label')).filter(el => el.getClientRects().length).every(el => el.scrollWidth <= el.clientWidth)`, "desktop labels fit alongside two-digit counts")
			}

			// Ordinary Tab order and visible focus work in both layouts.
			require.NoError(t, chromedp.Do(ctx, chromedp.Focus(chromedp.CSS(`.gallery-spaces a[href="/"]`)), chromedp.KeyEvent(kb.Tab)))
			if viewport.phone {
				check(`document.activeElement.getAttribute('href') === '/search'`, "Tab reaches Search second")
				require.NoError(t, chromedp.Do(ctx, chromedp.KeyEvent(kb.Tab)))
			}
			check(`document.activeElement.getAttribute('href') === '/shared' && document.activeElement.matches(':focus-visible') && getComputedStyle(document.activeElement).outlineStyle === 'solid'`, "Tab reaches the incoming space with a visible outline")
			require.NoError(t, chromedp.Do(ctx, chromedp.KeyEvent(kb.Tab)))
			check(`document.activeElement.getAttribute('href') === '/shared-by-me'`, "Tab reaches outgoing shares next")
			err = chromedp.Do(ctx, chromedp.KeyEvent(kb.Enter), chromedp.Poll[chromedp.Void](`location.pathname === "/shared-by-me"`))
			require.NoError(t, err)
			check(`location.pathname === '/shared-by-me' && document.querySelector('.gallery-spaces [aria-current="page"]').getAttribute('href') === '/shared-by-me'`, "keyboard activation navigates to outgoing shares")
			for _, path := range []string{"/", "/shared"} {
				// Load can finish before pagereveal starts the cross-document
				// transition, when getAnimations() is still empty. Wait for the
				// first frame and its transition so the snapshot cannot eat a click.
				require.NoError(t, chromedp.Do(ctx,
					chromedp.Evaluate[chromedp.Void](`new Promise(resolve => requestAnimationFrame(resolve)).then(() => document.activeViewTransition?.finished)`, chromedp.EvalAwaitPromise),
					chromedp.Poll[chromedp.Void](`document.getAnimations().length === 0`)))
				err = chromedp.Do(ctx, chromedp.Click(chromedp.CSS(`.gallery-spaces a[href="`+path+`"]`)), chromedp.Poll[chromedp.Void](`location.pathname === "`+path+`"`))
				require.NoError(t, err)
				check(`location.pathname === '`+path+`' && document.querySelector('.gallery-spaces [aria-current="page"]').getAttribute('href') === '`+path+`'`, "space links navigate and update the active space")
			}
			_, err = chromedp.Call(ctx, emulation.SetEmulatedMedia, emulation.SetEmulatedMediaParams{Features: []*emulation.MediaFeature{{Name: "prefers-color-scheme", Value: "dark"}}})
			require.NoError(t, err)
			check(`matchMedia('(prefers-color-scheme: dark)').matches && getComputedStyle(document.querySelector('.gallery-rail')).backgroundColor === getComputedStyle(document.querySelector('header')).backgroundColor`, "the navigation follows the dark panel token")
		})
	}
}

// The zero-count destinations use the same htmx navigation and read-only
// boundary as populated spaces, including after the final grant is revoked.
func TestGalleryEmptySpacesBrowser(t *testing.T) {
	in := newTwoOwnerInstance(t)
	app := httptest.NewServer(in.ro)
	defer app.Close()
	in.ro.cfg.AppOrigin = app.URL
	browser, err := chromium.New(chromiumtest.Start(t))
	require.NoError(t, err)
	ctx, cancel, err := browser.Tab(context.Background())
	require.NoError(t, err)
	defer cancel()
	ctx, cancel = context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	_, err = chromedp.Call(ctx, network.SetCookie, network.SetCookieParams{Name: in.cookieTwo.Name, Value: in.cookieTwo.Value, URL: app.URL, Path: "/"})
	require.NoError(t, err)
	check := func(js string) {
		t.Helper()
		ok, err := chromedp.Run(ctx, chromedp.Evaluate[bool](js))
		require.NoError(t, err)
		require.True(t, ok, js)
	}
	visit := func(path string) {
		t.Helper()
		require.NoError(t, chromedp.Do(ctx,
			chromedp.Click(chromedp.CSS(`.gallery-spaces a[href="`+path+`"]`)),
			chromedp.Poll[chromedp.Void](`location.pathname === '`+path+`' && document.querySelector('.gallery-spaces [aria-current]')?.getAttribute('href') === '`+path+`'`)))
	}
	for _, width := range []int64{320, 375, 390, 430, 1280} {
		t.Logf("viewport width: %d", width)
		_, err = chromedp.Call(ctx, emulation.SetDeviceMetricsOverride, emulation.SetDeviceMetricsOverrideParams{Width: width, Height: 844, DeviceScaleFactor: 1, Mobile: width < 768})
		require.NoError(t, err)
		require.NoError(t, chromedp.Do(ctx, chromedp.Navigate(app.URL), chromedp.WaitVisible(chromedp.CSS(`.gallery-spaces`))))
		require.NoError(t, chromedp.Do(ctx, chromedp.Evaluate[chromedp.Void](`window.originalHeader=document.querySelector('header')`)))
		for _, scheme := range []string{"light", "dark"} {
			_, err = chromedp.Call(ctx, emulation.SetEmulatedMedia, emulation.SetEmulatedMediaParams{Features: []*emulation.MediaFeature{{Name: "prefers-color-scheme", Value: scheme}}})
			require.NoError(t, err)
			visit("/shared")
			check(`document.querySelector('.empty').textContent === 'Nothing has been shared with you yet.' && document.querySelector('#gallery-content').dataset.galleryReadOnly === 'true' && !document.querySelector('[data-duplicate-dialog],.card-actions')`)
			check(`Array.from(document.querySelectorAll('.space-label')).map(el=>el.textContent).join('|') === 'Gallery|Search|Shared with me|Shared by me'`)
			check(`document.documentElement.scrollWidth <= innerWidth && Array.from(document.querySelectorAll('.gallery-spaces a')).filter(a => a.getClientRects().length).every(a=>a.getBoundingClientRect().width >= 44 && a.getBoundingClientRect().height >= 44)`)
			check(`Array.from(document.querySelectorAll('.space-label')).filter(el => el.getClientRects().length).every(el=>getComputedStyle(el).visibility === 'visible' && el.scrollWidth <= el.clientWidth && el.scrollHeight <= el.clientHeight)`)
			if dir := os.Getenv("GALLERY_SCREENSHOT_DIR"); dir != "" {
				require.NoError(t, os.MkdirAll(dir, 0755))
				shot, err := chromedp.Run(ctx, chromedp.Screenshot(chromedp.CSS("body")))
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(filepath.Join(dir, fmt.Sprintf("empty-%d-%s.png", width, scheme)), shot, 0644))
			}
			visit("/shared-by-me")
			check(`document.querySelector('.empty').textContent.includes('Open an artifact and choose Share.') && document.querySelector('#gallery-content').dataset.galleryReadOnly === 'false'`)
			visit("/")
			check(`document.querySelector('header') === originalHeader && document.querySelectorAll('.gallery-spaces a').length === 4`)
		}
		// Enlarge navigation text independently of viewport zoom, then exercise
		// a longer label. Its full text must remain inside its clickable link.
		require.NoError(t, chromedp.Do(ctx, chromedp.Evaluate[chromedp.Void](`document.querySelectorAll('.space-label').forEach(el=>el.style.fontSize='24px');document.querySelector('.space-label').textContent='My personal artifact library'`)))
		check(`document.documentElement.scrollWidth <= innerWidth && Array.from(document.querySelectorAll('.space-label')).filter(el => el.getClientRects().length).every(el=>{const r=el.getBoundingClientRect(),a=el.closest('a').getBoundingClientRect();return el.scrollWidth <= el.clientWidth && el.scrollHeight <= el.clientHeight && r.left >= a.left && r.right <= a.right && r.bottom <= a.bottom;})`)
	}
	// A newly received share appears on the next visit; revoking the last one
	// restores the explanation without stranding the user in that space.
	_, err = in.ro.cfg.Store.CreateGrant(ctx, in.ownerOne, in.artifactOne, in.ownerTwo)
	require.NoError(t, err)
	visit("/shared")
	check(`document.querySelectorAll('.card').length === 1 && !document.querySelector('.empty')`)
	require.NoError(t, in.ro.cfg.Store.DeleteGrant(ctx, in.ownerOne, in.artifactOne, in.ownerTwo))
	visit("/")
	visit("/shared")
	check(`document.querySelectorAll('.card').length === 0 && document.querySelector('.empty').textContent === 'Nothing has been shared with you yet.' && document.querySelectorAll('.gallery-spaces a').length === 4`)
}
