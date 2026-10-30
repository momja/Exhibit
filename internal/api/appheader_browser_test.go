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
		for _, path := range []string{"/", "/profile", "/new", "/admin/users", "/agent", "/not-a-page", "/artifacts/" + id + "/edit?return_to=/artifacts/" + id} {
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
