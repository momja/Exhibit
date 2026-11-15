package api

import (
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
	"github.com/momja/Exhibit/internal/chromium"
	"github.com/momja/Exhibit/internal/chromium/chromiumtest"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/require"
)

func TestGallerySpacesBrowser(t *testing.T) {
	for _, viewport := range []struct {
		name          string
		width, height int64
		phone         bool
	}{{"narrow", 320, 844, true}, {"phone", 390, 844, true}, {"tablet", 768, 900, false}, {"desktop", 1280, 900, false}} {
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
			checkSpaceLayout := func() {
				check(`(() => {
 const heading = document.querySelector('main h1');
 return !document.querySelector('.gallery-overview') && !document.querySelector('.gallery-title') && Boolean(heading?.classList.contains('sr-only')) && heading.textContent.trim().length > 0;
})()`, "the destination is named only for assistive technology")
				if viewport.phone {
					return
				}
				check(`Array.from(document.querySelectorAll('.space-label')).filter(label => label.getClientRects().length).every(label => {
 const text = document.createRange();
 text.selectNodeContents(label);
 return text.getClientRects().length === 1 && label.scrollWidth <= label.clientWidth;
})`, "desktop destination labels stay on one line alongside two-digit counts")
				check(`Array.from(document.querySelectorAll('.gallery-spaces a')).filter(a => a.getClientRects().length).every((a, index) => {
 const rect = a.getBoundingClientRect();
 return Math.abs(rect.height - 44) < 1 && (!window.spaceRowPositions || Math.abs(rect.top - window.spaceRowPositions[index]) < 1);
})`, "changing the active destination preserves navigation row heights and positions")
			}
			check(`document.querySelectorAll('nav[aria-label="Gallery spaces"]').length === 1 && document.querySelectorAll('.gallery-spaces [aria-current="page"]').length === 1 && document.querySelector('.gallery-spaces [aria-current="page"]').getAttribute('href') === '/shared'`, "one nav and the incoming space is active")
			check(`Array.from(document.querySelectorAll('.gallery-spaces a')).filter(a => a.getClientRects().length).every(a => a.getBoundingClientRect().width >= 44 && a.getBoundingClientRect().height >= 44) && document.documentElement.scrollWidth <= innerWidth`, "all targets are at least 44px and the page fits")
			check(fmt.Sprintf(`Array.from(document.querySelectorAll('.space-count')).every(el => (getComputedStyle(el).display === 'none') === %t)`, viewport.phone), "counts appear only in the rail")
			if viewport.phone {
				check(`getComputedStyle(document.querySelector('.gallery-rail')).paddingBottom === '38px' && document.querySelector('.gallery-rail').getBoundingClientRect().height >= 87 && document.querySelector('.gallery-rail').getBoundingClientRect().height <= 96`, "the compact tabs include the phone home-indicator inset")
				check(`getComputedStyle(document.querySelector('.gallery-rail')).position === 'fixed' && Math.abs(document.querySelector('.gallery-rail').getBoundingClientRect().bottom - innerHeight) < 1`, "phone tabs are pinned to the viewport bottom")
				require.NoError(t, chromedp.Do(ctx, chromedp.Evaluate[chromedp.Void](`window.scrollTo(0, document.documentElement.scrollHeight)`)))
				check(`Array.from(document.querySelectorAll('.card')).at(-1).getBoundingClientRect().bottom <= document.querySelector('.gallery-rail').getBoundingClientRect().top`, "the last card scrolls fully above the tabs")
			} else {
				check(`getComputedStyle(document.querySelector('.gallery-layout')).display === 'grid' && document.querySelector('.gallery-rail').getBoundingClientRect().right <= document.querySelector('main').getBoundingClientRect().left`, "desktop rail sits beside the grid")
				require.NoError(t, chromedp.Do(ctx, chromedp.Evaluate[chromedp.Void](`window.spaceRowPositions = Array.from(document.querySelectorAll('.gallery-spaces a')).filter(a => a.getClientRects().length).map(a => a.getBoundingClientRect().top)`)))
			}
			checkSpaceLayout()

			// Ordinary Tab order and visible focus work in both layouts.
			require.NoError(t, chromedp.Do(ctx, chromedp.Focus(chromedp.CSS(`.gallery-spaces a[href="/"]`)), chromedp.KeyEvent(kb.Tab)))
			check(`document.activeElement.getAttribute('href') === '/search'`, "Tab reaches Search second")
			require.NoError(t, chromedp.Do(ctx, chromedp.KeyEvent(kb.Tab)))
			check(`document.activeElement.getAttribute('href') === '/shared' && document.activeElement.matches(':focus-visible') && getComputedStyle(document.activeElement).outlineStyle === 'solid'`, "Tab reaches the incoming space with a visible outline")
			require.NoError(t, chromedp.Do(ctx, chromedp.KeyEvent(kb.Tab)))
			check(`document.activeElement.getAttribute('href') === '/activity'`, "Tab reaches Activity next")
			err = chromedp.Do(ctx, chromedp.KeyEvent(kb.Enter), chromedp.Poll[chromedp.Void](`location.pathname === "/activity"`))
			require.NoError(t, err)
			check(`location.pathname === '/activity' && document.querySelector('.gallery-spaces [aria-current="page"]').getAttribute('href') === '/activity'`, "keyboard activation navigates to Activity")
			checkSpaceLayout()
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
				checkSpaceLayout()
			}
			_, err = chromedp.Call(ctx, emulation.SetEmulatedMedia, emulation.SetEmulatedMediaParams{Features: []*emulation.MediaFeature{{Name: "prefers-color-scheme", Value: "dark"}}})
			require.NoError(t, err)
			check(`matchMedia('(prefers-color-scheme: dark)').matches && getComputedStyle(document.querySelector('.gallery-rail')).backgroundColor === getComputedStyle(document.querySelector('header')).backgroundColor`, "the navigation follows the dark panel token")
			checkSpaceLayout()
		})
	}
}

// The zero-count destinations use the same htmx navigation and read-only
// boundary as populated spaces, including after the final grant is revoked.
func TestGalleryEmptyPresentationBrowser(t *testing.T) {
	ro := newTestRouter(t)
	require.NoError(t, ro.cfg.Store.CreateTag(t.Context(), &store.Tag{ID: "empty-presentation", OwnerID: defaultOwnerID, Name: "Empty", Color: "#377a64"}))
	app := httptest.NewServer(ro)
	defer app.Close()
	ro.cfg.AppOrigin = app.URL
	browser, err := chromium.New(chromiumtest.Start(t))
	require.NoError(t, err)
	ctx, cancel, err := browser.Tab(context.Background())
	require.NoError(t, err)
	defer cancel()
	ctx, cancel = context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	for _, width := range []int64{1280, 768, 390, 320} {
		_, err = chromedp.Call(ctx, emulation.SetDeviceMetricsOverride, emulation.SetDeviceMetricsOverrideParams{Width: width, Height: 844, DeviceScaleFactor: 1, Mobile: width < 768})
		require.NoError(t, err)
		for _, scheme := range []string{"light", "dark"} {
			_, err = chromedp.Call(ctx, emulation.SetEmulatedMedia, emulation.SetEmulatedMediaParams{Features: []*emulation.MediaFeature{{Name: "prefers-color-scheme", Value: scheme}}})
			require.NoError(t, err)
			for _, state := range []struct{ path, heading, action string }{
				{"/", "No starred artifacts yet", "/search"},
				{"/shared", "Nothing shared yet", ""},
				{"/search", "Welcome to Exhibit", "/new"},
				{"/search?q=missing", "No artifacts match your search.", "/search"},
				{"/search?tag=Empty", "No artifacts match your filters.", "/search"},
			} {
				t.Logf("%d %s %s", width, scheme, state.path)
				require.NoError(t, chromedp.Do(ctx, chromedp.Navigate(app.URL+state.path), chromedp.WaitVisible(chromedp.CSS(`.gallery-spaces`)), chromedp.Evaluate[chromedp.Void](`document.fonts.ready`, chromedp.EvalAwaitPromise)))
				ok, err := chromedp.Run(ctx, chromedp.Evaluate[bool](fmt.Sprintf(`(() => {
 const state = document.querySelector('.gallery-empty');
 const heading = state?.querySelector('h2.empty'), helper = state?.querySelector('.empty-hint'), action = state?.querySelector('a');
 if (!heading || !helper || heading.textContent !== %q || !helper.textContent.trim()) return false;
 const titleStyle = getComputedStyle(heading), helperStyle = getComputedStyle(helper);
 return titleStyle.fontWeight === '600' && helperStyle.fontWeight === '400' && titleStyle.textTransform === 'none'
   && helper.getBoundingClientRect().top >= heading.getBoundingClientRect().bottom + 8
   && (!action || Math.abs(action.getBoundingClientRect().height - 44) < 1)
   && (action?.getAttribute('href') || '') === %q
   && matchMedia('(prefers-color-scheme: dark)').matches === %t
   && document.documentElement.scrollWidth <= innerWidth;
})()`, state.heading, state.action, scheme == "dark")))
				require.NoError(t, err)
				require.True(t, ok, "empty destinations keep a readable heading/helper hierarchy and standard action")
				if strings.Contains(state.path, "?") {
					require.NoError(t, chromedp.Do(ctx, chromedp.Evaluate[chromedp.Void](`window.emptyHeader=document.querySelector('header')`), chromedp.Click(chromedp.CSS(`.gallery-empty a`)), chromedp.Poll[chromedp.Void](`location.pathname === '/search' && !location.search && document.querySelector('.gallery-welcome')`)))
					ok, err = chromedp.Run(ctx, chromedp.Evaluate[bool](`document.querySelector('header') === emptyHeader && document.querySelector('.gallery-empty a').getAttribute('href') === '/new'`))
					require.NoError(t, err)
					require.True(t, ok, "clearing a miss restores the server-rendered welcome without replacing the header")
				}
			}
		}
	}
}

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
			check(`document.querySelector('.empty').textContent === 'Nothing shared yet' && document.querySelector('.empty-hint').textContent === 'Share one of your artifacts, or ask someone to share with you.' && document.querySelector('#gallery-content').dataset.galleryReadOnly === 'false' && !document.querySelector('.card-actions')`)
			check(`Array.from(document.querySelectorAll('.space-label')).map(el=>el.textContent).join('|') === 'Starred|Search|Shared|Activity'`)
			check(`document.documentElement.scrollWidth <= innerWidth && Array.from(document.querySelectorAll('.gallery-spaces a')).filter(a => a.getClientRects().length).every(a=>a.getBoundingClientRect().width >= 44 && a.getBoundingClientRect().height >= 44)`)
			check(`Array.from(document.querySelectorAll('.space-label')).filter(el => el.getClientRects().length).every(el=>getComputedStyle(el).visibility === 'visible' && el.scrollWidth <= el.clientWidth && el.scrollHeight <= el.clientHeight)`)
			if dir := os.Getenv("GALLERY_SCREENSHOT_DIR"); dir != "" {
				require.NoError(t, os.MkdirAll(dir, 0755))
				shot, err := chromedp.Run(ctx, chromedp.Screenshot(chromedp.CSS("body")))
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(filepath.Join(dir, fmt.Sprintf("empty-%d-%s.png", width, scheme)), shot, 0644))
			}
			visit("/activity")
			check(`document.querySelector('.activity-placeholder').textContent.includes('Coming soon') && document.querySelector('#gallery-content').dataset.galleryReadOnly === 'false'`)
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
	check(`document.querySelectorAll('.card').length === 0 && document.querySelector('.empty').textContent === 'Nothing shared yet' && document.querySelector('.empty-hint').textContent === 'Share one of your artifacts, or ask someone to share with you.' && document.querySelectorAll('.gallery-spaces a').length === 4`)
}
