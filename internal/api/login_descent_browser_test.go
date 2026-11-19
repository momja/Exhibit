package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/momja/Exhibit/internal/chromium"
	"github.com/momja/Exhibit/internal/chromium/chromiumtest"
	"github.com/stretchr/testify/require"
)

// The descent (Exh-vsw0) in a real browser: the form is usable and focused on
// the first frame and never moves; the layers settle exactly where the composite
// image is drawn; and a visitor who asked for no motion gets no animation and
// downloads none of the layer art.
// awaitLoginStyled returns once the form exists and every stylesheet in the page
// has been parsed, without reading any style. Reading computed style (as
// WaitVisible does) while a head stylesheet is still loading caches the unstyled
// value, and the sheet then applies as a 300ms opacity transition: a state no
// visitor sees, because the browser withholds first paint until head sheets load.
// Assertions on the first state must start from here.
func awaitLoginStyled(t *testing.T, ctx context.Context) {
	t.Helper()
	require.NoError(t, chromedp.Do(ctx, chromedp.Poll[chromedp.Void](`document.querySelector('#username') !== null && [...document.querySelectorAll('link[rel=stylesheet]')].every(l => l.sheet)`, chromedp.WithPollingTimeout(10*time.Second))))
}

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
			require.NoError(t, chromedp.Do(ctx, chromedp.Navigate(app.URL+"/auth/login")))
			awaitLoginStyled(t, ctx)
			eval := func(script string) bool {
				t.Helper()
				ok, err := chromedp.Run(ctx, chromedp.Evaluate[bool](script))
				require.NoError(t, err, script)
				return ok
			}
			require.True(t, eval(`document.body.classList.contains('login-descend')`), "the first visit is marked to animate in both cases")
			require.True(t, eval(`document.activeElement.id === 'username'`), "the form has focus on the first frame")
			require.True(t, eval(`document.querySelector('.login-headline .sr-only').textContent === 'tools'`), "the accessible headline stays fixed")
			if motion == "no-preference" {
				require.True(t, eval(`(() => {
					const track = document.querySelector('.login-headline-words');
					const animation = track.getAnimations()[0];
					const slot = track.parentElement;
					const slotHeight = slot.getBoundingClientRect().height;
					const frameMatches = (time, row) => {
						animation.currentTime = time + animation.effect.getTiming().delay;
						const offset = new DOMMatrix(getComputedStyle(track).transform).m42;
						return Math.abs(offset + slotHeight * row) < 0.5;
					};
					const originalTime = animation.currentTime;
					const matches = frameMatches(10000, 0) && frameMatches(13000, 1) &&
						frameMatches(16000, 2) && frameMatches(19000, 3) && frameMatches(21001, 0) &&
						frameMatches(31000, 0) && frameMatches(34000, 1);
					animation.currentTime = originalTime;
					return matches && animation.effect.getTiming().iterations === Infinity &&
						animation.effect.getTiming().delay === -10000;
				})()`), "the carousel loops, with a longer rest on tools between passes")
				require.True(t, eval(`(() => {
					const slot = document.querySelector('.login-headline-window');
					const feather = slot.getAnimations()[0];
					const originalTime = feather.currentTime;
					const at = time => {
						feather.currentTime = time + feather.effect.getTiming().delay;
						return getComputedStyle(slot).getPropertyValue('--login-word-feather');
					};
					const matches = at(10000) === '0%' && at(12450) === '24%' && at(13000) === '0%';
					feather.currentTime = originalTime;
					return matches;
				})()`), "the carousel edges fade only during transitions")
			}
			formRect := `JSON.stringify(document.querySelector('main.login').getBoundingClientRect())`
			before, err := chromedp.Run(ctx, chromedp.Evaluate[string](formRect))
			require.NoError(t, err)
			layers := `performance.getEntriesByType('resource').filter(e => /-(sky|clouds|hill)-/.test(e.name)).length`
			if motion == "reduce" {
				time.Sleep(500 * time.Millisecond)
				require.True(t, eval(`document.getAnimations().length === 0`), "no motion")
				require.True(t, eval(layers+` === 0`), "no layer art is downloaded")
				require.True(t, eval(`getComputedStyle(document.querySelector('.login-scene-composite')).opacity === '1'`), "the composite is the page")
				return
			}
			require.NoError(t, chromedp.Do(ctx, chromedp.Poll[chromedp.Void](layers+` >= 3`, chromedp.WithPollingTimeout(10*time.Second))))
			require.True(t, eval(`document.getAnimations().filter(a => a.transitionProperty).length === 0`), "no opacity fade runs on an animating visit")
			require.True(t, eval(`getComputedStyle(document.querySelector('.login-scene-composite')).opacity === '0'`), "an animating visit does not start on the finished picture")
			// Jump to the end of every animation, as the timeline would.
			require.NoError(t, chromedp.Do(ctx, chromedp.Poll[chromedp.Void](`document.body.classList.contains('login-descent-go')`, chromedp.WithPollingTimeout(5*time.Second))), "the animation starts once the layers are decoded")
			require.True(t, eval(`document.querySelector('.login-scene').getAnimations({subtree:true}).forEach(a => a.finish()) || true`))
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
	_, err = chromedp.Call(ctx, emulation.SetEmulatedMedia, emulation.SetEmulatedMediaParams{Features: []*emulation.MediaFeature{{Name: "prefers-reduced-motion", Value: "no-preference"}}})
	require.NoError(t, err)
	// Do not wait for load: it waits for the layers this test holds back.
	_, err = chromedp.Call(ctx, page.Navigate, page.NavigateParams{URL: app.URL + "/auth/login"})
	require.NoError(t, err)
	awaitLoginStyled(t, ctx)
	ok, err := chromedp.Run(ctx, chromedp.Evaluate[bool](`getComputedStyle(document.querySelector('.login-scene-composite')).opacity === '0' && !document.body.classList.contains('login-descent-skip')`))
	require.NoError(t, err)
	require.True(t, ok, "the visit starts on the plain sky while the layers are awaited")
	time.Sleep(2500 * time.Millisecond)
	ok, err = chromedp.Run(ctx, chromedp.Evaluate[bool](`getComputedStyle(document.querySelector('.login-scene-composite')).opacity === '1' && !document.body.classList.contains('login-descent-go') && document.querySelectorAll('.login-layers-stage img').length === 0 && document.getAnimations().filter(a => a.animationName.startsWith('login-descent')).length === 0`))
	require.NoError(t, err)
	require.True(t, ok)
}

// Reduced motion switched on mid-run cancels the descent for good: the layers
// go, the composite is shown, and switching back does not restart anything.
func TestLoginDescentCancelledByReducedMotionMidRun(t *testing.T) {
	ro, _ := newLocalLoginRouter(t)
	app := httptest.NewServer(ro)
	defer app.Close()
	ro.cfg.AppOrigin = app.URL
	browser, err := chromium.New(chromiumtest.Start(t))
	require.NoError(t, err)
	ctx, cancel, err := browser.Tab(context.Background())
	require.NoError(t, err)
	defer cancel()
	ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	motion := func(value string) {
		_, err := chromedp.Call(ctx, emulation.SetEmulatedMedia, emulation.SetEmulatedMediaParams{Features: []*emulation.MediaFeature{{Name: "prefers-reduced-motion", Value: value}}})
		require.NoError(t, err)
	}
	motion("no-preference")
	require.NoError(t, chromedp.Do(ctx, chromedp.Navigate(app.URL+"/auth/login"), chromedp.WaitVisible(chromedp.CSS(`#username`))))
	require.NoError(t, chromedp.Do(ctx, chromedp.Poll[chromedp.Void](`document.body.classList.contains('login-descent-go')`, chromedp.WithPollingTimeout(5*time.Second))))
	motion("reduce")
	require.NoError(t, chromedp.Do(ctx, chromedp.Poll[chromedp.Void](`document.querySelectorAll('.login-layers-stage img').length === 0 && document.body.classList.contains('login-descent-skip')`, chromedp.WithPollingTimeout(5*time.Second))))
	motion("no-preference")
	time.Sleep(500 * time.Millisecond)
	ok, err := chromedp.Run(ctx, chromedp.Evaluate[bool](`!document.body.classList.contains('login-descent-go') && document.querySelectorAll('.login-layers-stage img').length === 0 && getComputedStyle(document.querySelector('.login-scene-composite')).opacity === '1' && document.getAnimations().filter(a => a.animationName.startsWith('login-descent')).length === 0`))
	require.NoError(t, err)
	require.True(t, ok, "a cancelled descent never restarts")
}
