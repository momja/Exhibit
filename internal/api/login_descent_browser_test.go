package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/chromedp"
	"github.com/momja/Exhibit/internal/chromium"
	"github.com/momja/Exhibit/internal/chromium/chromiumtest"
	"github.com/stretchr/testify/require"
)

// The descent (Exh-vsw0) in a real browser: the form is usable and focused on
// the first frame and never moves; the layers settle exactly where the composite
// image is drawn; and a visitor who asked for no motion gets no animation and
// downloads none of the layer art.
func TestLoginDescentBrowser(t *testing.T) {
	for _, motion := range []string{"no-preference", "reduce"} {
		t.Run(motion, func(t *testing.T) {
			ro, _ := newLocalLoginRouter(t)
			app := httptest.NewServer(ro)
			defer app.Close()
			ro.cfg.AppOrigin = app.URL
			browser, err := chromium.New(chromiumtest.Start(t))
			require.NoError(t, err)
			ctx, cancel, err := browser.Tab(context.Background())
			require.NoError(t, err)
			defer cancel()
			ctx, cancel = context.WithTimeout(ctx, 40*time.Second)
			defer cancel()
			_, err = chromedp.Call(ctx, emulation.SetDeviceMetricsOverride, emulation.SetDeviceMetricsOverrideParams{Width: 1280, Height: 800, DeviceScaleFactor: 1})
			require.NoError(t, err)
			_, err = chromedp.Call(ctx, emulation.SetEmulatedMedia, emulation.SetEmulatedMediaParams{Features: []*emulation.MediaFeature{{Name: "prefers-reduced-motion", Value: motion}}})
			require.NoError(t, err)
			require.NoError(t, chromedp.Do(ctx, chromedp.Navigate(app.URL+"/auth/login"), chromedp.WaitVisible(chromedp.CSS(`#username`))))
			eval := func(script string) bool {
				t.Helper()
				ok, err := chromedp.Run(ctx, chromedp.Evaluate[bool](script))
				require.NoError(t, err, script)
				return ok
			}
			require.True(t, eval(`document.body.classList.contains('login-descend')`), "the first visit is marked to animate in both cases")
			require.True(t, eval(`document.activeElement.id === 'username'`), "the form has focus on the first frame")
			formRect := `JSON.stringify(document.querySelector('main.login').getBoundingClientRect())`
			before, err := chromedp.Run(ctx, chromedp.Evaluate[string](formRect))
			require.NoError(t, err)
			layers := `performance.getEntriesByType('resource').filter(e => /-(sky|clouds|hill)-/.test(e.name)).length`
			if motion == "reduce" {
				time.Sleep(500 * time.Millisecond)
				require.True(t, eval(`document.getAnimations().length === 0`), "no motion")
				require.True(t, eval(layers+` === 0`), "no layer art is downloaded")
				require.True(t, eval(`getComputedStyle(document.querySelector('.login-scene-composite')).visibility === 'visible'`), "the composite is the page")
				return
			}
			require.NoError(t, chromedp.Do(ctx, chromedp.Poll[chromedp.Void](layers+` >= 3`, chromedp.WithPollingTimeout(10*time.Second))))
			// Jump to the end of every animation, as the timeline would.
			require.NoError(t, chromedp.Do(ctx, chromedp.Poll[chromedp.Void](`document.body.classList.contains('login-descent-go')`, chromedp.WithPollingTimeout(5*time.Second))), "the animation starts once the layers are decoded")
			require.True(t, eval(`document.getAnimations().forEach(a => a.finish()) || true`))
			// The hill layer fills the stage, and the stage is where the
			// composite's object-fit:cover; object-position:78% 50% puts it.
			require.True(t, eval(`(() => {
				const vw = innerWidth, vh = innerHeight, s = Math.max(vw / 2560, vh / 1440);
				const want = { x: (vw - 2560 * s) * .78, y: (vh - 1440 * s) * .5, w: 2560 * s, h: 1440 * s };
				const r = document.querySelector('.login-layer-hill').getBoundingClientRect();
				const sky = document.querySelector('.login-layer-sky').getBoundingClientRect();
				const near = (a, b) => Math.abs(a - b) < .6;
				return near(r.x, want.x) && near(r.y, want.y) && near(r.width, want.w) && near(r.height, want.h) && near(sky.y, want.y - 832 * s) && near(sky.height, 2272 * s);
			})()`), "layers end exactly on the composite's placement")
			require.True(t, eval(`getComputedStyle(document.querySelector('.login-scene-composite')).visibility === 'visible'`), "the composite is never hidden")
			after, err := chromedp.Run(ctx, chromedp.Evaluate[string](formRect))
			require.NoError(t, err)
			require.Equal(t, before, after, "the form never moved")
		})
	}
}

// If the layers cannot be had in time, the page stays on the static composite
// and never starts the animation.
func TestLoginDescentSkippedWhenLayersAreLate(t *testing.T) {
	ro, _ := newLocalLoginRouter(t)
	// The sky layer takes far longer than the deadline.
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "-sky-") {
			time.Sleep(4 * time.Second)
		}
		ro.ServeHTTP(w, r)
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
	require.NoError(t, chromedp.Do(ctx, chromedp.Navigate(app.URL+"/auth/login"), chromedp.WaitVisible(chromedp.CSS(`#username`))))
	time.Sleep(2500 * time.Millisecond)
	ok, err := chromedp.Run(ctx, chromedp.Evaluate[bool](`!document.body.classList.contains('login-descent-go') && document.querySelectorAll('.login-layers-stage img').length === 0 && document.getAnimations().length === 0`))
	require.NoError(t, err)
	require.True(t, ok)
}
