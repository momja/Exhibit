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

	for _, width := range []int64{1280, 768, 767, 600, 390, 320} {
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
							if (el.matches('.gallery-filter-trigger')) {
								const label = el.querySelector('span').getBoundingClientRect();
								if (label.left < box.left || label.right > box.right) return false;
							}
							return box.left >= rect.left && box.right <= rect.right && box.top >= rect.top && box.bottom <= rect.bottom;
						})};
				})()`))
				require.NoError(t, err)
				height, padding := 37.0, "16px"
				if width <= 767 {
					height = 33
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
// coverage; these cases exercise the hidden recovery and failure layout.
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
		w.Header().Set("Content-Type", "text/html")
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
	for _, width := range []int64{1280, 390} {
		_, err := chromedp.Call(ctx, emulation.SetDeviceMetricsOverride, emulation.SetDeviceMetricsOverrideParams{Width: width, Height: 900, DeviceScaleFactor: 1})
		require.NoError(t, err)
		for _, path := range []string{"/settings", "/gallery"} {
			t.Run(fmt.Sprintf("%d%s", width, path), func(t *testing.T) {
				require.NoError(t, chromedp.Do(ctx, chromedp.Navigate(app.URL+path)))
				hidden, err := chromedp.Run(ctx, chromedp.Evaluate[bool](`(() => {
					const recovery = document.querySelector('[data-clerk-target="recovery"]');
					return recovery.hidden && getComputedStyle(recovery).display === 'none' && recovery.getClientRects().length === 0;
				})()`))
				require.NoError(t, err)
				require.True(t, hidden, "successful account loading must not reveal the recovery link")
				require.NoError(t, chromedp.Do(ctx, chromedp.Evaluate[chromedp.Void](`
					document.querySelector('[data-clerk-target="status"]').textContent = 'The account service did not respond. Reload this page to retry. If this continues, check whether your browser or network is blocking Clerk.';
					document.querySelector('[data-clerk-target="recovery"]').hidden = false;
				`)))
				fits, err := chromedp.Run(ctx, chromedp.Evaluate[bool](`(() => {
					const header = document.querySelector('.app-header').getBoundingClientRect();
					const status = document.querySelector('[data-clerk-target="status"]').getBoundingClientRect();
					const recovery = document.querySelector('[data-clerk-target="recovery"]').getBoundingClientRect();
					return status.top >= header.top && status.bottom <= header.bottom &&
						recovery.width > 0 && recovery.top >= header.top && recovery.bottom <= header.bottom &&
						document.querySelector('main').getBoundingClientRect().top >= header.bottom &&
						document.documentElement.scrollWidth <= innerWidth;
				})()`))
				require.NoError(t, err)
				require.True(t, fits, "account-service errors remain visible inside the bar and above page content")
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
				return rect.height === (innerWidth <= 767 ? 33 : 37) && rect.width === innerWidth &&
					!header.querySelector('a, button, [data-controller="clerk"]') &&
					document.documentElement.scrollWidth <= innerWidth && document.querySelector('main').getBoundingClientRect().top >= rect.bottom;
			})()`))
			require.NoError(t, err)
			require.True(t, ok, "%d /%s: auth uses the shared bar without authenticated controls", width, mode)
		}
	}
}

func TestFloatingAppHeaderBrowser(t *testing.T) {
	ro := newTestRouter(t)
	id := createArtifact(t, ro, map[string]any{"title": "Floating header", "body": "<!doctype html><h1>Tool</h1>"})
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
	ctx, cancel = context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	run := func(actions ...chromedp.Action[chromedp.Void]) {
		t.Helper()
		require.NoError(t, chromedp.Do(ctx, actions...))
	}
	wait := func(expression string) { t.Helper(); run(chromedp.Poll[chromedp.Void](expression)) }
	run(chromedp.Navigate(app.URL))
	for _, width := range []int64{1280, 390} {
		_, err := chromedp.Call(ctx, emulation.SetDeviceMetricsOverride, emulation.SetDeviceMetricsOverrideParams{Width: width, Height: 900, DeviceScaleFactor: 1})
		require.NoError(t, err)
		run(chromedp.Evaluate[chromedp.Void](`localStorage.removeItem('exhibit.pipControls')`), chromedp.Navigate(app.URL+"/artifacts/"+id), chromedp.Evaluate[chromedp.Void](fmt.Sprintf(`import(%q).then(({application}) => window.headerApplication = application)`, assetURL("gallery/controllers.js"))))
		wait(`window.headerApplication?.getControllerForElementAndIdentifier(document.querySelector('[data-app-header]'), 'floating-header') != null`)
		run(chromedp.Click(chromedp.CSS(`#pip-more`)))
		wait(`!document.getElementById('pip-menu').hidden`)
		run(chromedp.KeyEvent(kb.Escape))
		wait(`document.getElementById('pip-menu').hidden && document.activeElement.id === 'pip-more'`)
		run(chromedp.Click(chromedp.CSS(`#pip-title`)))
		wait(`!document.getElementById('pip-info').hidden && document.activeElement.id === 'pip-info'`)
		run(chromedp.KeyEvent(kb.Escape), chromedp.Focus(chromedp.CSS(`#pip-grip`)), chromedp.KeyEvent(kb.ArrowRight))
		wait(`document.body.dataset.pipCorner === 'br'`)
		run(chromedp.KeyEvent(kb.ArrowRight))
		wait(`!document.getElementById('pip-tab').hidden && document.activeElement.id === 'pip-tab'`)
		run(chromedp.KeyEvent(kb.Enter))
		wait(`!document.getElementById('pip').hidden && document.activeElement.id === 'pip-grip'`)
		run(chromedp.Evaluate[chromedp.Void](`window.savedHeader = document.querySelector('[data-app-header]'); window.savedHeader.remove()`))
		wait(`window.headerApplication.getControllerForElementAndIdentifier(window.savedHeader, 'floating-header') == null`)
		run(chromedp.Evaluate[chromedp.Void](`window.savedHeader.querySelector('#pip-more').click()`))
		closed, err := chromedp.Run(ctx, chromedp.Evaluate[bool](`window.savedHeader.querySelector('#pip-menu').hidden`))
		require.NoError(t, err)
		require.True(t, closed, "disconnected controls release their listeners")
		run(chromedp.Evaluate[chromedp.Void](`document.body.append(window.savedHeader)`))
		wait(`window.headerApplication.getControllerForElementAndIdentifier(window.savedHeader, 'floating-header') != null`)
		run(chromedp.Click(chromedp.CSS(`#pip-more`)))
		wait(`!document.getElementById('pip-menu').hidden`)
	}
}
