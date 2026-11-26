package api

import (
	"context"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
	"github.com/momja/Exhibit/internal/chromium"
	"github.com/momja/Exhibit/internal/chromium/chromiumtest"
	"github.com/stretchr/testify/require"
)

// Exercise actual page styles together: a page sheet must not change the
// shared bar's dimensions, even when the gallery nests it under a controller.
func TestAppHeaderBrowser(t *testing.T) {
	in := newProfileInstance(t, "header-admin", "header@example.test")
	seedGalleryFilters(t, in.ro, in.user.ID)
	id := createArtifact(t, in.ro, map[string]any{
		"title": "An artifact with a very long title that must fit the return link on a phone without hiding the save action",
		"body":  "<!doctype html><h1>Header</h1>",
	})
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.AddCookie(in.cookie)
		in.ro.ServeHTTP(w, r)
	}))
	defer app.Close()
	in.ro.cfg.AppOrigin = app.URL

	browser, err := chromium.New(chromiumtest.Start(t))
	require.NoError(t, err)
	ctx, cancel, err := browser.Tab(context.Background())
	require.NoError(t, err)
	defer cancel()
	ctx, cancel = context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	for _, width := range []int64{1280, 768, 767, 600, 430, 390, 375, 320} {
		_, err = chromedp.Call(ctx, emulation.SetDeviceMetricsOverride, emulation.SetDeviceMetricsOverrideParams{Width: width, Height: 900, DeviceScaleFactor: 1})
		require.NoError(t, err)
		for _, path := range []string{"/", "/profile", "/connections", "/connections?advanced=1&artifact=" + id, "/new", "/admin/users", "/agent", "/not-a-page", "/artifacts/" + id + "/edit?return_to=/artifacts/" + id} {
			t.Run(fmt.Sprintf("%d%s", width, path), func(t *testing.T) {
				require.NoError(t, chromedp.Do(ctx, chromedp.Navigate(app.URL+path), chromedp.Poll[chromedp.Void](`document.querySelector('.app-header')`)))
				metrics, err := chromedp.Run(ctx, chromedp.Evaluate[struct {
					Height  float64 `json:"height"`
					Padding string  `json:"padding"`
					Fits    bool    `json:"fits"`
				}](`(() => {
					const header = document.querySelector('.app-header');
					const rect = header.getBoundingClientRect();
					return {height: rect.height, padding: getComputedStyle(header).paddingLeft,
						fits: document.documentElement.scrollWidth <= innerWidth &&
						[...header.querySelectorAll('a, button, input')].filter(el => el.getBoundingClientRect().width > 0).every(el => {
							const box = el.getBoundingClientRect();
							if (el.matches('.gallery-filter-trigger') && innerWidth > 767) {
								const label = el.querySelector('span').getBoundingClientRect();
								if (label.left < box.left || label.right > box.right) return false;
							}
							if (innerWidth <= 767 && (box.width < 44 || box.height < 44)) return false;
							return box.left >= rect.left && box.right <= rect.right && box.top >= rect.top && box.bottom <= rect.bottom;
						})};
				})()`))
				require.NoError(t, err)
				height, padding := 56.0, "16px"
				if width <= 767 {
					height = 56
				}
				if width <= 360 {
					padding = "8px"
				}
				require.Equal(t, height, metrics.Height)
				require.Equal(t, padding, metrics.Padding)
				require.True(t, metrics.Fits, "header controls and page stay within the viewport")
				if path == "/agent" && width == 1280 {
					require.NoError(t, chromedp.Do(ctx,
						chromedp.Evaluate[chromedp.Void](`window.closeKeyModal()`),
						chromedp.Click(chromedp.CSS(`#key-btn`)),
						chromedp.Poll[chromedp.Void](`document.getElementById('key-modal').hidden === false`),
						chromedp.Evaluate[chromedp.Void](`window.closeKeyModal()`),
					))
				}
			})
		}
	}
}

// Use the actual header partial and page styles without loading the remote
// provider. Provider mounting and auth behavior have their own integration
// coverage; these cases exercise the first HTML and anchored failure layout.
func TestAppHeaderAccountStatesBrowser(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("/assets/", assetsHandler())
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		header, stylesheet := profileHeader(&clerkView{}), "settings.css"
		if r.URL.Path == "/gallery" {
			header, stylesheet = galleryHeader(galleryGridData{ID: "library-gallery"}, true, &clerkView{}), "index.css"
		}
		markup, err := renderPage("appHeader", header)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<!doctype html><html><head><meta name="viewport" content="width=device-width,initial-scale=1"><link rel="stylesheet" href="/assets/gallery/tokens.css"><link rel="stylesheet" href="/assets/gallery/components.css"><link rel="stylesheet" href="/assets/gallery/%s"></head><body>%s<main>Page content</main></body></html>`, stylesheet, markup)
	})
	app := httptest.NewServer(mux)
	defer app.Close()
	browser, err := chromium.New(chromiumtest.Start(t))
	require.NoError(t, err)
	ctx, cancel, err := browser.Tab(context.Background())
	require.NoError(t, err)
	defer cancel()
	ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	for _, width := range []int64{1280, 430, 390, 375, 320} {
		_, err := chromedp.Call(ctx, emulation.SetDeviceMetricsOverride, emulation.SetDeviceMetricsOverrideParams{Width: width, Height: 900, DeviceScaleFactor: 1})
		require.NoError(t, err)
		for _, path := range []string{"/settings", "/gallery"} {
			t.Run(fmt.Sprintf("%d%s", width, path), func(t *testing.T) {
				require.NoError(t, chromedp.Do(ctx, chromedp.Navigate(app.URL+path)))
				hidden, err := chromedp.Run(ctx, chromedp.Evaluate[bool](`(() => {
					const recovery = document.querySelector('[data-clerk-target="recovery"]');
					const placeholder = document.querySelector('.account-control-placeholder');
					return recovery.hidden && getComputedStyle(recovery).display === 'none' && recovery.getClientRects().length === 0 &&
						placeholder.getBoundingClientRect().width === 44 && placeholder.getBoundingClientRect().height === 44 &&
						document.querySelector('[role="status"]').textContent === 'Loading your account…';
				})()`))
				require.NoError(t, err)
				require.True(t, hidden, "without JavaScript the first HTML reserves space, describes loading, and hides recovery actions")
				// Model the SDK's compact avatar trigger without a remote account.
				require.NoError(t, chromedp.Do(ctx, chromedp.Evaluate[chromedp.Void](`window.headerBeforeReady = document.querySelector('.app-header').getBoundingClientRect().toJSON(); document.querySelector('.account-control').dataset.clerkState = 'ready'; document.querySelector('[data-clerk-target="status"]').textContent = ''; document.querySelector('[data-clerk-target="mount"]').innerHTML = '<button style="width:28px;height:28px" aria-label="Account">A</button>'`)))
				if width <= 767 {
					ok, err := chromedp.Run(ctx, chromedp.Evaluate[bool](`(() => {const r=document.querySelector('[data-clerk-target="mount"] button').getBoundingClientRect();return r.width>=44 && r.height>=44 && document.documentElement.scrollWidth<=innerWidth})()`))
					require.NoError(t, err)
					require.True(t, ok, "the hosted account trigger has a full mobile hit box")
				}
				stable, err := chromedp.Run(ctx, chromedp.Evaluate[bool](`JSON.stringify(document.querySelector('.app-header').getBoundingClientRect().toJSON()) === JSON.stringify(headerBeforeReady) && getComputedStyle(document.querySelector('.account-control-placeholder')).display === 'none' && getComputedStyle(document.querySelector('[data-clerk-target="mount"]')).visibility === 'visible'`))
				require.NoError(t, err)
				require.True(t, stable, "the reserved account slot prevents layout shifts when its button appears")
				require.NoError(t, chromedp.Do(ctx, chromedp.Evaluate[chromedp.Void](`
					window.headerBeforeFailure = document.querySelector('.app-header').getBoundingClientRect().toJSON();
					// Initial SDK load failure never mounts an avatar.
					document.querySelector('[data-clerk-target="mount"]').replaceChildren();
					document.querySelector('.account-control').dataset.clerkState = 'error';
					document.querySelector('[data-clerk-target="status"]').classList.remove('sr-only');
					document.querySelector('[data-clerk-target="status"]').textContent = 'The account service did not respond. Reload this page to retry. If this continues, check whether your browser or network is blocking Clerk.';
					document.querySelector('[data-clerk-target="recovery"]').hidden = false;
				`)))
				fits, err := chromedp.Run(ctx, chromedp.Evaluate[bool](`(() => {
					const header = document.querySelector('.app-header').getBoundingClientRect();
					const status = document.querySelector('[data-clerk-target="status"]').getBoundingClientRect();
					const recovery = document.querySelector('[data-clerk-target="recovery"]').getBoundingClientRect();
					const search = document.querySelector('[data-gallery-query]');
					return status.top >= header.bottom && status.left >= 0 && status.right <= innerWidth &&
						(!search || !search.getClientRects().length || search.getBoundingClientRect().width >= 80) &&
						recovery.width > 0 && recovery.top >= status.bottom && recovery.right <= innerWidth &&
						JSON.stringify(header.toJSON()) === JSON.stringify(headerBeforeFailure) &&
						document.querySelector('main').getBoundingClientRect().top >= header.bottom &&
						document.documentElement.scrollWidth <= innerWidth;
				})()`))
				require.NoError(t, err)
				require.True(t, fits, "account-service errors stay visible below the bar without moving the header or page content")
			})
		}
	}
}

// A new full-document app page must opt into the same component. Fragments
// and render-origin artifact documents are deliberately outside this shell.
func TestEveryAppPageUsesSharedHeader(t *testing.T) {
	require.NoError(t, fs.WalkDir(templatesFS, "templates", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		source, err := templatesFS.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(source), "<!DOCTYPE html>") {
			require.Contains(t, string(source), `{{template "appHeader" .Header}}`, path)
		}
		return nil
	}))
}

func TestAuthAppHeaderBrowser(t *testing.T) {
	ro := newTestRouter(t)
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			ro.ServeHTTP(w, r)
			return
		}
		if r.URL.Path == "/local" {
			ro.renderLogin(w, r, http.StatusOK, "/", "", "")
			return
		}
		// Exercise the production hosted template without loading a remote SDK.
		ro.renderClerkLogin(w, r, strings.TrimPrefix(r.URL.Path, "/"))
	}))
	defer app.Close()
	ro.cfg.AppOrigin = app.URL
	browser, err := chromium.New(chromiumtest.Start(t))
	require.NoError(t, err)
	ctx, cancel, err := browser.Tab(context.Background())
	require.NoError(t, err)
	defer cancel()
	ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	for _, width := range []int64{1280, 390, 320} {
		_, err := chromedp.Call(ctx, emulation.SetDeviceMetricsOverride, emulation.SetDeviceMetricsOverrideParams{Width: width, Height: 900, DeviceScaleFactor: 1})
		require.NoError(t, err)
		for _, mode := range []string{"local", "signin", "signup", "logout"} {
			require.NoError(t, chromedp.Do(ctx, chromedp.Navigate(app.URL+"/"+mode)))
			ok, err := chromedp.Run(ctx, chromedp.Evaluate[bool](`(() => {
				const header = document.querySelector('[data-app-header]');
				const rect = header.getBoundingClientRect();
				return rect.height === (innerWidth <= 767 ? 56 : 56) && rect.width === innerWidth &&
					!header.querySelector('a, button, [data-controller="clerk"]') &&
					document.documentElement.scrollWidth <= innerWidth && document.querySelector('main').getBoundingClientRect().top >= rect.bottom;
			})()`))
			require.NoError(t, err)
			require.True(t, ok, "%d /%s: auth uses the shared bar without authenticated controls", width, mode)
		}
	}
}

// barTestArtifact is a tall page that scrolls itself, focuses, and reports
// what it sees, all on the test's request through postMessage: the render
// frame is cross-origin, so the host page cannot reach into it.
const barTestArtifact = `<!doctype html>%s
<style>body{margin:0}.row{height:60px;border-bottom:1px solid #ccc}</style>
<input id="top" aria-label="top"><div id="rows"></div><button id="last">Last</button><div id="after"></div>
<script>
for (let i = 0; i < 60; i++) { const d = document.createElement('div'); d.className = 'row'; document.getElementById(i < 30 ? 'rows' : 'after').append(d); }
function report(extra) {
  const root = document.documentElement;
  parent.postMessage({ barReport: Object.assign({
    bar: getComputedStyle(root).getPropertyValue('--exhibit-bar').trim(),
    padding: root.style.scrollPaddingBottom, height: innerHeight }, extra) }, '*');
}
addEventListener('message', e => {
  const d = e.data || {};
  if (d.cmd === 'scroll') scrollTo(0, d.y);
  if (d.cmd === 'focusTop') document.getElementById('top').focus();
  if (d.cmd === 'report') report({});
});
document.getElementById('last').addEventListener('focus', function () {
  requestAnimationFrame(() => report({ lastBottom: this.getBoundingClientRect().bottom }));
});
</script>`

// Exh-i8q4 (docs/artifact-bar.md §7): the artifact bar at the bottom centre,
// a real scroll in a real render frame compacting it, the band resizing the
// frame, the overlay variable reaching the frame, and the accessibility
// requirements of §6 that need a browser to show.
func TestFloatingAppHeaderBrowser(t *testing.T) {
	ro := newTestRouter(t)
	bandID := createArtifact(t, ro, map[string]any{"title": "Floating header", "body": fmt.Sprintf(barTestArtifact, "")})
	overlayID := createArtifact(t, ro, map[string]any{"title": "Edge to edge", "body": fmt.Sprintf(barTestArtifact, `<meta name="exhibit-bar" content="overlay">`)})
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
	run := func(actions ...chromedp.Action[chromedp.Void]) {
		t.Helper()
		require.NoError(t, chromedp.Do(ctx, actions...))
	}
	wait := func(expression, message string) {
		t.Helper()
		pollCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		require.NoError(t, chromedp.Do(pollCtx, chromedp.Poll[chromedp.Void](expression)), message)
	}
	eval := func(expression string) { t.Helper(); run(chromedp.Evaluate[chromedp.Void](expression)) }
	open := func(id string) {
		t.Helper()
		run(chromedp.Navigate(app.URL+"/artifacts/"+id), chromedp.Evaluate[chromedp.Void](fmt.Sprintf(`import(%q).then(({application}) => window.headerApplication = application)`, assetURL("gallery/controllers.js"))))
		wait(`window.headerApplication?.getControllerForElementAndIdentifier(document.querySelector('[data-app-header]'), 'floating-header') != null`, "the controller connects")
		eval(`window.barReports = []; addEventListener('message', e => e.data && e.data.barReport && barReports.push(e.data.barReport))`)
		// The frame is lazy and its preamble announces the mode once parsed.
		wait(`document.querySelector('.panels > iframe').contentWindow && (postToFrame = m => document.querySelector('.panels > iframe').contentWindow.postMessage(m, '*'), postToFrame({cmd: 'report'}), barReports.length > 0)`, "the artifact frame is running")
	}
	const (
		// The bar's gap to the window bottom is 12px full and 4px compact, the
		// band is that gap on either side of the bar, and the frame ends where
		// the band begins: so the bar never covers the artifact.
		geometry = `(compact => { const bar = document.getElementById('pip').getBoundingClientRect();
			const frame = document.querySelector('.panels > iframe').getBoundingClientRect(); const gap = compact ? 4 : 12;
			return document.getElementById('pip').classList.contains('pip-compact') === compact &&
				Math.abs(bar.left + bar.width / 2 - innerWidth / 2) < 1 && Math.abs(innerHeight - bar.bottom - gap) < 1 &&
				(!document.body.classList.contains('pip-inset') || Math.abs(frame.bottom - (bar.top - gap)) < 1); })`
		compactNow = `document.getElementById('pip').classList.contains('pip-compact')`
	)

	for _, size := range []struct{ width, height int64 }{{1280, 800}, {390, 844}} {
		{
			_, err := chromedp.Call(ctx, emulation.SetDeviceMetricsOverride, emulation.SetDeviceMetricsOverrideParams{Width: size.width, Height: size.height, DeviceScaleFactor: 1})
			require.NoError(t, err)
			run(chromedp.Navigate(app.URL), chromedp.Evaluate[chromedp.Void](`localStorage.setItem('exhibit.pipControls', '{"mode":"corner","corner":"tr"}')`))
			open(bandID)
			wait(`localStorage.getItem('exhibit.pipControls') === null`, "the pill's saved placement is cleared")
			wait(geometry+`(false)`, "the full bar sits at the bottom centre above a band the frame ends at")
			if size.width < 768 {
				wait(`(() => { const bar = document.getElementById('pip').getBoundingClientRect(); return Math.abs(bar.left - 12) < 1 && Math.abs(innerWidth - bar.right - 12) < 1; })()`, "a phone's bar spans the width")
			}

			// Popups keep their keyboard behaviour.
			run(chromedp.Click(chromedp.CSS(`#pip-more`)))
			wait(`!document.getElementById('pip-menu').hidden && document.getElementById('pip-menu').getBoundingClientRect().bottom <= document.getElementById('pip').getBoundingClientRect().top`, "the menu opens upward")
			run(chromedp.KeyEvent(kb.Escape))
			wait(`document.getElementById('pip-menu').hidden && document.activeElement.id === 'pip-more'`, "Escape returns focus")
			run(chromedp.Click(chromedp.CSS(`#pip-title`)))
			wait(`!document.getElementById('pip-info').hidden && document.activeElement.id === 'pip-info'`, "the title opens the card")
			run(chromedp.KeyEvent(kb.Escape))
			eval(`document.activeElement.blur()`)

			// A real scroll in the render frame compacts the bar, and the band follows it.
			eval(`postToFrame({cmd: 'scroll', y: 600})`)
			wait(geometry+`(true)`, "scrolling down compacts the bar and the band shrinks with it")
			eval(`postToFrame({cmd: 'scroll', y: 300})`)
			wait(geometry+`(false)`, "scrolling up brings the full bar back")
			eval(`postToFrame({cmd: 'scroll', y: 600})`)
			wait(compactNow, "compact again")

			// §6.1: a click with no pointer press behind it (assistive
			// technology) expands the bar and also opens the card.
			eval(`document.getElementById('pip-title').click()`)
			wait(`!`+compactNow+` && !document.getElementById('pip-info').hidden`, "a focus-less click on the compact title opens the card")
			run(chromedp.KeyEvent(kb.Escape))
			eval(`document.activeElement.blur()`)

			// §6.1: a touch tap on the strip only expands it.
			eval(`postToFrame({cmd: 'scroll', y: 300}); setTimeout(() => postToFrame({cmd: 'scroll', y: 900}), 150)`)
			wait(geometry+`(true)`, "compact again")
			var strip struct{ X, Y float64 }
			strip, err = chromedp.Run(ctx, chromedp.Evaluate[struct{ X, Y float64 }](`(() => { const r = document.getElementById('pip').getBoundingClientRect(); return {X: r.left + r.width / 2, Y: r.top + r.height / 2}; })()`))
			require.NoError(t, err)
			_, err = chromedp.Call(ctx, emulation.SetTouchEmulationEnabled, emulation.SetTouchEmulationEnabledParams{Enabled: true, MaxTouchPoints: 1})
			require.NoError(t, err)
			_, err = chromedp.Call(ctx, input.SynthesizeTapGesture, input.SynthesizeTapGestureParams{X: strip.X, Y: strip.Y, GestureSourceType: input.GestureSourceTypeTouch})
			require.NoError(t, err)
			_, err = chromedp.Call(ctx, emulation.SetTouchEmulationEnabled, emulation.SetTouchEmulationEnabledParams{Enabled: false})
			require.NoError(t, err)

			wait(`!`+compactNow, "a tap expands the strip")
			ok, err := chromedp.Run(ctx, chromedp.Evaluate[bool](`new Promise(r => setTimeout(() => r(document.getElementById('pip-info').hidden), 200))`, chromedp.EvalAwaitPromise))
			require.NoError(t, err)
			require.True(t, ok, "and does not also open the info card under the finger")

			// §6.1: Tab into the compact bar expands it.
			eval(`postToFrame({cmd: 'scroll', y: 300}); setTimeout(() => postToFrame({cmd: 'scroll', y: 1200}), 150)`)
			wait(compactNow, "compact again")
			eval(`document.activeElement.blur(); document.body.focus()`)
			run(chromedp.KeyEvent(kb.Tab))
			wait(`document.getElementById('pip').contains(document.activeElement) && !`+compactNow, "a keyboard user never meets a control they cannot see")
			eval(`document.activeElement.blur()`)

			// §6.4: with a 200% root font size the bar grows and the band follows it.
			eval(`document.documentElement.style.fontSize = '200%'`)
			wait(geometry+`(false) && document.getElementById('pip').getBoundingClientRect().height > 80`, "larger text grows the full bar and its band")
			eval(`postToFrame({cmd: 'scroll', y: 300}); setTimeout(() => postToFrame({cmd: 'scroll', y: 1500}), 150)`)
			wait(geometry+`(true) && Math.abs(document.getElementById('pip').getBoundingClientRect().height - 60) < 1`, "and the compact one")
			eval(`document.documentElement.style.fontSize = ''`)

			// Disconnecting releases the controller; reconnecting restores it.
			eval(`window.savedHeader = document.querySelector('[data-app-header]'); window.savedHeader.remove()`)
			wait(`window.headerApplication.getControllerForElementAndIdentifier(window.savedHeader, 'floating-header') == null`, "disconnects")
			wait(`!document.body.classList.contains('pip-inset')`, "the disposer removes the band")
			eval(`window.savedHeader.querySelector('#pip-more').click()`)
			closed, err := chromedp.Run(ctx, chromedp.Evaluate[bool](`window.savedHeader.querySelector('#pip-menu').hidden`))
			require.NoError(t, err)
			require.True(t, closed, "disconnected controls release their listeners")
			eval(`document.body.append(window.savedHeader)`)
			wait(`window.headerApplication.getControllerForElementAndIdentifier(window.savedHeader, 'floating-header') != null`, "reconnects")
			run(chromedp.Click(chromedp.CSS(`#pip-more`)))
			wait(`!document.getElementById('pip-menu').hidden`, "and works")

			// The overlay opt-in: a full-height frame told the bar's height.
			open(overlayID)
			wait(`!document.body.classList.contains('pip-inset') && Math.abs(document.querySelector('.panels > iframe').getBoundingClientRect().bottom - innerHeight) < 1`, "an opted-in artifact gets the whole window")
			wait(`(postToFrame({cmd: 'report'}), barReports.at(-1).bar === (innerHeight - document.getElementById('pip').getBoundingClientRect().top + 12) + 'px')`, "--exhibit-bar reaches the frame as the bar's height")
			wait(`barReports.at(-1).padding === 'var(--exhibit-bar, 0px)'`, "the root's scroll padding keeps focused controls clear of the bar")
			eval(`postToFrame({cmd: 'scroll', y: 600})`)
			wait(compactNow+` && (postToFrame({cmd: 'report'}), barReports.at(-1).bar === '38px')`, "and follows the compact bar")

			// §6.2: a control focused by keyboard at the bottom is scrolled clear of the bar.
			eval(`document.querySelector('.panels > iframe').focus(); postToFrame({cmd: 'scroll', y: 0}); postToFrame({cmd: 'focusTop'})`)
			wait(`document.activeElement === document.querySelector('.panels > iframe')`, "focus is in the artifact")
			run(chromedp.KeyEvent(kb.Tab))
			wait(`(r => r && r.lastBottom !== undefined && r.lastBottom <= r.height - parseFloat(r.bar) + 1)(barReports.at(-1))`, "the focused button clears the bar")
		}
	}
}

// Phones pin the bar to the viewport and the slot reserves the height it gives up (Exh-b6r6).
func TestPhoneHeaderPinnedBrowser(t *testing.T) {
	in := newProfileInstance(t, "header-pinned", "pinned@example.test")
	id := createArtifact(t, in.ro, map[string]any{
		"title": "A page tall enough to scroll",
		"body":  "<!doctype html><h1>Pinned header</h1>",
	})
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.AddCookie(in.cookie)
		in.ro.ServeHTTP(w, r)
	}))
	defer app.Close()
	in.ro.cfg.AppOrigin = app.URL

	browser, err := chromium.New(chromiumtest.Start(t))
	require.NoError(t, err)
	ctx, cancel, err := browser.Tab(context.Background())
	require.NoError(t, err)
	defer cancel()
	ctx, cancel = context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	const (
		// Out of flow: fixed at the viewport's top edge and full width.
		pinned = `(() => {
			const header = document.querySelector('.app-header');
			const slot = document.querySelector('.app-header-slot');
			const bar = header.getBoundingClientRect(), held = slot.getBoundingClientRect();
			const main = document.querySelector('main').getBoundingClientRect();
			return getComputedStyle(header).position === 'fixed' && Math.abs(bar.top) < 1 &&
				Math.abs(bar.width - innerWidth) < 1 && Math.abs(held.height - bar.height) < 1 &&
				main.top >= bar.bottom - 1 &&
				parseFloat(getComputedStyle(slot).getPropertyValue('--app-header-height')) > 0; })()`
		// Desktop keeps the bar in the flow: the slot draws no box at all.
		inFlow = `(() => {
			const header = document.querySelector('.app-header');
			const slot = document.querySelector('.app-header-slot');
			return getComputedStyle(header).position === 'sticky' && getComputedStyle(slot).display === 'contents' &&
				slot.getClientRects().length === 0; })()`
		// The slot's box is what replaces the bar in the flow.
		slotBox = `getComputedStyle(document.querySelector('.app-header-slot')).display === 'block'`
	)
	poll := func(script, message string) {
		t.Helper()
		pollCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		require.NoError(t, chromedp.Do(pollCtx, chromedp.Poll[chromedp.Void](script)), message)
	}
	eval := func(script string) {
		t.Helper()
		require.NoError(t, chromedp.Do(ctx, chromedp.Evaluate[chromedp.Void](script)))
	}

	// A page shorter than the viewport and one that scrolls: the two shapes overscroll produces.
	for _, width := range []int64{390, 320} {
		_, err := chromedp.Call(ctx, emulation.SetDeviceMetricsOverride, emulation.SetDeviceMetricsOverrideParams{Width: width, Height: 700, DeviceScaleFactor: 1, Mobile: true})
		require.NoError(t, err)
		for _, path := range []string{"/profile", "/artifacts/" + id + "/edit?return_to=/artifacts/" + id} {
			t.Run(fmt.Sprintf("%d%s", width, path), func(t *testing.T) {
				require.NoError(t, chromedp.Do(ctx, chromedp.Navigate(app.URL+path), chromedp.Poll[chromedp.Void](`document.querySelector('.app-header-slot')`)))
				poll(slotBox, "the slot draws a box at phone widths, which is what holds the page's place")
				poll(pinned, "the bar holds its place and the page content clears it")
				eval(`window.scrollTo(0, document.scrollingElement.scrollHeight)`)
				poll(`Math.abs(document.querySelector('.app-header').getBoundingClientRect().top) < 1`, "the bar stays at the top of the viewport at the end of the page")
				eval(`window.scrollTo(0, 0)`)
				poll(pinned, "back at the top the bar is where the slot reserved it")
			})
		}
	}

	_, err = chromedp.Call(ctx, emulation.SetDeviceMetricsOverride, emulation.SetDeviceMetricsOverrideParams{Width: 1280, Height: 800, DeviceScaleFactor: 1})
	require.NoError(t, err)
	require.NoError(t, chromedp.Do(ctx, chromedp.Navigate(app.URL+"/artifacts/"+id+"/edit?return_to=/artifacts/"+id), chromedp.Poll[chromedp.Void](inFlow)), "desktop keeps the sticky bar with no slot box")
	eval(`window.scrollTo(0, 400)`)
	poll(`window.scrollY > 0 && Math.abs(document.querySelector('.app-header').getBoundingClientRect().top) < 1`, "the sticky bar still holds at the top once the page scrolls")
}
