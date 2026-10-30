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
		for _, path := range []string{"/", "/profile", "/new", "/admin/users", "/artifacts/" + id + "/edit?return_to=/artifacts/" + id} {
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
			})
		}
	}
}
