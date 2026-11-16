package api

import (
	"context"
	"io/fs"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/chromedp"
	"github.com/momja/Exhibit/internal/agent"
	"github.com/momja/Exhibit/internal/chromium"
	"github.com/momja/Exhibit/internal/chromium/chromiumtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The agent's working indicator is a pencil that boils (Exh-s8yu). Its frames
// are baked by the icon build; web/icons/boil.test.mjs holds the bake to being
// deterministic and to the embedded bytes being a fresh bake.
func TestIconBakeSuite(t *testing.T) {
	runNodeTestSuites(t, "icons", "boil.test.mjs")
}

// The page carries the whole indicator, frames included, in a <template> that
// agent.js clones: the script holds no path data and builds no markup.
func TestAgentPageCarriesTheBoilingIndicator(t *testing.T) {
	r := newTestRouter(t)
	page := getPage(t, r, "/agent")

	_, rest, ok := strings.Cut(page, `<template id="agent-thinking-template">`)
	require.True(t, ok, "the agent page carries the thinking template")
	tmpl, _, ok := strings.Cut(rest, `</template>`)
	require.True(t, ok)

	svg, err := fs.ReadFile(assetsFS(), "phosphor/pencil-simple-boil.svg")
	require.NoError(t, err)
	assert.Equal(t,
		`<div class="thinking is-boiling">`+strings.TrimSpace(string(svg))+`thinking…</div>`, tmpl,
		"the baked SVG, inlined verbatim beside the unchanged label")
	for _, frame := range []string{`class="f f1"`, `class="f f2"`, `class="f f3"`} {
		assert.Equal(t, 1, strings.Count(tmpl, frame), frame)
	}
	assert.Equal(t, 3, strings.Count(tmpl, "<path "), "exactly three frames")
	assert.Contains(t, tmpl, `fill="currentColor"`, "the icon takes the text colour")
}

func TestInlineSVGRefusesWhatItCannotInline(t *testing.T) {
	_, err := inlineSVG("phosphor/regular.css")
	assert.ErrorContains(t, err, "not an SVG")
	_, err = inlineSVG("phosphor/no-such-icon.svg")
	assert.ErrorContains(t, err, "make assets")
}

// Exactly one frame is in view at any moment, in either colour scheme and with
// Reduce Motion on (av-90ej: the indicator keeps boiling). What counts is what
// is drawn: a frame is in view when its box overlaps the SVG's. The sweep
// pauses the strip and steps it through two whole cycles, with probes either
// side of every step; the live pass watches it run for real.
func TestBoilingIndicatorBrowser(t *testing.T) {
	ro := newTestRouter(t)
	// Platform mode: a BYOK page with no key opens the key dialog over the chat.
	enablePlatformMode(ro)
	ro.cfg.Agent = &agent.Manager{}
	app := httptest.NewServer(ro)
	defer app.Close()
	ro.cfg.AppOrigin = app.URL
	browser, err := chromium.New(chromiumtest.Start(t))
	require.NoError(t, err)
	ctx, cancel, err := browser.Tab(context.Background())
	require.NoError(t, err)
	defer cancel()
	ctx, cancel = context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	evaluate := func(expression string) string {
		t.Helper()
		out, err := chromedp.Run(ctx, chromedp.Evaluate[string](expression, chromedp.EvalAwaitPromise))
		require.NoError(t, err)
		return out
	}
	shots := os.Getenv("BOIL_SCREENSHOT_DIR")

	colours := map[string]string{}
	for _, scheme := range []string{"light", "dark"} {
		for _, motion := range []string{"no-preference", "reduce"} {
			t.Logf("%s, reduced motion %s", scheme, motion)
			_, err := chromedp.Call(ctx, emulation.SetEmulatedMedia, emulation.SetEmulatedMediaParams{Features: []*emulation.MediaFeature{
				{Name: "prefers-color-scheme", Value: scheme}, {Name: "prefers-reduced-motion", Value: motion}}})
			require.NoError(t, err)
			require.NoError(t, chromedp.Do(ctx, chromedp.EmulateViewport(480, 320), chromedp.Navigate(app.URL+"/agent")))
			require.NoError(t, chromedp.Do(ctx, chromedp.Poll[chromedp.Void](`typeof window.showPane === 'function'`)))

			// The same clone agent.js makes on thinking_start.
			assert.Equal(t, "ok", evaluate(`(() => {
				const el = document.getElementById('agent-thinking-template').content.firstElementChild.cloneNode(true);
				document.getElementById('messages').append(el);
				const svg = el.querySelector('svg.boil'), frames = [...svg.querySelectorAll('.f')];
				const inView = () => {
					const box = svg.getBoundingClientRect();
					return frames.filter(f => {
						const r = f.getBoundingClientRect();
						return Math.min(r.right, box.right) - Math.max(r.left, box.left) > 0.01;
					}).map(f => f.classList[1]);
				};
				const strip = svg.querySelector('.boil-strip');
				window.boil = { el, strip, inView };
				const box = svg.getBoundingClientRect();
				if (box.width !== 13 || box.height !== 13) return 'size ' + box.width + 'x' + box.height;
				if (el.textContent !== 'thinking…') return 'label ' + el.textContent;
				el.classList.remove('is-boiling');
				const still = inView().join('+');
				el.classList.add('is-boiling');
				return still === 'f1' ? 'ok' : 'not boiling, it shows ' + still;
			})()`))

			assert.Equal(t, "[]", evaluate(`(() => {
				const { strip, inView } = window.boil;
				const all = strip.getAnimations();
				if (all.length !== 1 || all[0].playState !== 'running') return 'not boiling';
				const [boil] = all;
				boil.pause();
				const times = [];
				for (let t = 0; t <= 750; t += 0.5) times.push(t);
				for (let b = 125; b <= 750; b += 125) times.push(b - 1e-3, b - 1e-6, b + 1e-6, b + 1e-3);
				const wrong = [];
				for (const t of times) {
					boil.currentTime = t;
					const shown = inView();
					if (shown.length !== 1) wrong.push(t + 'ms:' + (shown.join('+') || 'none'));
				}
				boil.currentTime = 0;
				boil.play();
				return JSON.stringify(wrong);
			})()`), "exactly one frame in view at every point of the cycle")

			live := evaluate(`new Promise(resolve => {
				const { inView } = window.boil;
				const seen = new Set(), wrong = [];
				const end = performance.now() + 1500;
				(function sample() {
					const shown = inView();
					if (shown.length !== 1) wrong.push(shown.join('+') || 'none');
					else seen.add(shown[0]);
					if (performance.now() < end) requestAnimationFrame(sample);
					else resolve(wrong.length ? 'wrong ' + wrong : [...seen].sort().join(','));
				})();
			})`)
			assert.Equal(t, "f1,f2,f3", live, "running for real, it shows one frame at a time and cycles all three")

			// fill="currentColor": the pencil is drawn in the label's colour.
			colour := evaluate(`(() => {
				const { el } = window.boil;
				const fill = getComputedStyle(el.querySelector('.f')).fill, ink = getComputedStyle(el).color;
				return fill === ink ? ink : 'fill ' + fill + ' is not ' + ink;
			})()`)
			assert.True(t, strings.HasPrefix(colour, "rgb"), colour)
			colours[scheme] = colour

			if shots != "" && motion == "no-preference" {
				captureBoilFrames(t, ctx, shots, scheme)
			}
		}
	}
	assert.NotEqual(t, colours["light"], colours["dark"], "the colour follows the scheme")
}

// captureBoilFrames saves the chat with each frame pinned, at 1x, 2x and 3x,
// for review (BOIL_SCREENSHOT_DIR).
func captureBoilFrames(t *testing.T, ctx context.Context, dir, scheme string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	_, err := chromedp.Run(ctx, chromedp.Evaluate[bool](`(() => {
		const user = document.createElement('div');
		user.className = 'msg user';
		user.textContent = 'Make me a pomodoro timer';
		document.getElementById('messages').prepend(user);
		return true;
	})()`))
	require.NoError(t, err)
	for _, scale := range []float64{1, 2, 3} {
		_, err := chromedp.Call(ctx, emulation.SetDeviceMetricsOverride, emulation.SetDeviceMetricsOverrideParams{Width: 480, Height: 320, DeviceScaleFactor: scale})
		require.NoError(t, err)
		for frame := 0; frame < 3; frame++ {
			// The middle of each frame's third of the 375ms cycle.
			_, err := chromedp.Run(ctx, chromedp.Evaluate[bool](`(() => {
				const [a] = window.boil.strip.getAnimations();
				a.pause();
				a.currentTime = `+[]string{"62", "187", "312"}[frame]+`;
				return true;
			})()`))
			require.NoError(t, err)
			shot, err := chromedp.Run(ctx, chromedp.Screenshot(chromedp.CSS(`#messages`)))
			require.NoError(t, err)
			name := scheme + "-" + []string{"1x", "2x", "3x"}[int(scale)-1] + "-f" + string(rune('1'+frame)) + ".png"
			require.NoError(t, os.WriteFile(filepath.Join(dir, name), shot, 0o644))
		}
	}
}
