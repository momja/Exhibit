package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
	"github.com/momja/Exhibit/internal/chromium"
	"github.com/momja/Exhibit/internal/chromium/chromiumtest"
	"github.com/stretchr/testify/require"
)

func TestNewPageChooserBrowser(t *testing.T) {
	r := newTestRouter(t)
	app := httptest.NewServer(r)
	defer app.Close()
	r.cfg.AppOrigin = app.URL
	browser, err := chromium.New(chromiumtest.Start(t))
	require.NoError(t, err)
	ctx, cancel, err := browser.Tab(context.Background())
	require.NoError(t, err)
	defer cancel()
	ctx, timeout := context.WithTimeout(ctx, 90*time.Second)
	defer timeout()

	for _, width := range []int64{320, 375, 390, 430, 1280} {
		for _, scheme := range []string{"light", "dark"} {
			t.Run(fmt.Sprintf("%d/%s", width, scheme), func(t *testing.T) {
				_, err := chromedp.Call(ctx, emulation.SetDeviceMetricsOverride, emulation.SetDeviceMetricsOverrideParams{Width: width, Height: 844, DeviceScaleFactor: 1})
				require.NoError(t, err)
				_, err = chromedp.Call(ctx, emulation.SetEmulatedMedia, emulation.SetEmulatedMediaParams{Features: []*emulation.MediaFeature{{Name: "prefers-color-scheme", Value: scheme}}})
				require.NoError(t, err)
				require.NoError(t, chromedp.Do(ctx, chromedp.Navigate(app.URL+"/new")))
				for _, mode := range []string{"agent", "paste", "url"} {
					require.NoError(t, chromedp.Do(ctx, chromedp.Click(chromedp.CSS(`.route[data-mode="`+mode+`"]`))))
					fits, err := chromedp.Run(ctx, chromedp.Evaluate[bool](`(() => {
						const routes = [...document.querySelectorAll('.route')];
						const selected = document.querySelector('.route[aria-pressed="true"]');
						if (document.querySelectorAll('.route[aria-pressed="true"]').length !== 1) return false;
						if (document.documentElement.scrollWidth > innerWidth) return false;
						if (!routes.every(el => {const r = el.getBoundingClientRect(); return r.width >= 44 && r.height >= 44})) return false;
						const note = document.querySelector('#route-description');
						if (innerWidth > 640) return !note.getClientRects().length && routes.every(el => el.querySelector('.route-note').getClientRects().length);
						const panel = document.querySelector('.ingest-panel:not([hidden])');
						const action = panel.querySelector('.ingest-foot button').getBoundingClientRect();
						return routes.every(el => !el.querySelector('.route-note').getClientRects().length) &&
							note.textContent === selected.querySelector('.route-note').textContent &&
							note.getBoundingClientRect().top >= document.querySelector('.routes').getBoundingClientRect().bottom &&
							panel.getBoundingClientRect().top < 280 && action.bottom < innerHeight &&
							[...panel.querySelectorAll('.field,.btn,.snapshot-row')].filter(el => el.getClientRects().length).every(el => el.getBoundingClientRect().height >= 44);
					})()`))
					require.NoError(t, err)
					require.True(t, fits, "%s: compact choices, one explanation and reachable action", mode)
					if dir := os.Getenv("NEW_PAGE_SCREENSHOT_DIR"); dir != "" {
						require.NoError(t, os.MkdirAll(dir, 0755))
						shot, err := chromedp.Run(ctx, chromedp.Screenshot(chromedp.CSS(`body`)))
						require.NoError(t, err)
						require.NoError(t, os.WriteFile(filepath.Join(dir, fmt.Sprintf("%d-%s-%s.png", width, scheme, mode)), shot, 0644))
					}
				}
				if width > 640 {
					return
				}
				// Stress the actual page with long labels and twice its computed text
				// sizes, without shrinking touch targets or clipping the page to hide overflow.
				require.NoError(t, chromedp.Do(ctx, chromedp.Evaluate[chromedp.Void](`
					document.querySelector('[data-mode="agent"] .route-label-compact').textContent = 'Build with an assistant';
					document.querySelector('[data-mode="paste"] .route-label-compact').textContent = 'Paste saved HTML source';
					document.querySelector('[data-mode="url"] .route-label-compact').textContent = 'Import from a website address';
					const sizes = [...document.querySelectorAll('main, main *')].map(el => [el, parseFloat(getComputedStyle(el).fontSize)]);
					for (const [el, size] of sizes) el.style.fontSize = size * 2 + 'px';
				`)))
				inset := int64(34)
				_, err = chromedp.Call(ctx, emulation.SetSafeAreaInsetsOverride, emulation.SetSafeAreaInsetsOverrideParams{Insets: &emulation.SafeAreaInsets{Bottom: &inset}})
				require.NoError(t, err)
				for _, mode := range []string{"agent", "paste", "url"} {
					require.NoError(t, chromedp.Do(ctx,
						chromedp.Click(chromedp.CSS(`.route[data-mode="`+mode+`"]`)),
						chromedp.Evaluate[chromedp.Void](`window.scrollTo(0, document.documentElement.scrollHeight)`)))
					fits, err := chromedp.Run(ctx, chromedp.Evaluate[bool](`(() => {
						const action = document.querySelector('.ingest-panel:not([hidden]) .ingest-foot button').getBoundingClientRect();
						return document.documentElement.scrollWidth <= innerWidth && action.top >= 0 && action.bottom <= innerHeight - 34 &&
							[...document.querySelectorAll('.route')].every(el => el.scrollWidth <= el.clientWidth);
					})()`))
					require.NoError(t, err)
					require.True(t, fits, "%s: enlarged text wraps and the action scrolls above the safe area", mode)
				}
				zero := int64(0)
				_, err = chromedp.Call(ctx, emulation.SetSafeAreaInsetsOverride, emulation.SetSafeAreaInsetsOverrideParams{Insets: &emulation.SafeAreaInsets{Bottom: &zero}})
				require.NoError(t, err)
			})
		}
	}
}

func TestNewPageModeSwitchBrowser(t *testing.T) {
	for _, width := range []int64{390, 1280} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			r := newTestRouter(t)
			payloads := make(chan map[string]any, 2)
			app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				if req.Method == http.MethodPost && req.URL.Path == "/api/artifacts" {
					var payload map[string]any
					if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
						http.Error(w, err.Error(), http.StatusBadRequest)
						return
					}
					payloads <- payload
					// Keep the form mounted to verify values survive a server refusal.
					http.Error(w, "Test ingest refusal", http.StatusBadRequest)
					return
				}
				if req.URL.Path == "/agent" {
					w.Header().Set("Content-Type", "text/html; charset=utf-8")
					fmt.Fprint(w, `<main id="agent-handoff">Agent handoff</main>`)
					return
				}
				r.ServeHTTP(w, req)
			}))
			defer app.Close()
			r.cfg.AppOrigin = app.URL
			browser, err := chromium.New(chromiumtest.Start(t))
			require.NoError(t, err)
			ctx, cancel, err := browser.Tab(context.Background())
			require.NoError(t, err)
			defer cancel()
			ctx, timeout := context.WithTimeout(ctx, 30*time.Second)
			defer timeout()
			run := func(actions ...chromedp.Action[chromedp.Void]) {
				t.Helper()
				require.NoError(t, chromedp.Do(ctx, actions...))
			}
			check := func(js string) {
				t.Helper()
				ok, err := chromedp.Run(ctx, chromedp.Evaluate[bool](js))
				require.NoError(t, err)
				require.True(t, ok, js)
			}
			run(chromedp.EmulateViewport(width, 844), chromedp.Navigate(app.URL+"/new"))
			check(`!document.querySelector('#agent-panel').hidden && !document.querySelector('.cm-editor') && document.querySelector('[data-mode="agent"]').getAttribute('aria-pressed') === 'true'`)
			run(chromedp.Click(chromedp.CSS(`#agent-panel button[type="submit"]`)))
			check(`location.pathname === '/new' && document.activeElement.id === 'agent-description' && !sessionStorage.getItem('exhibit:agent-brief')`)
			run(chromedp.SendKeys(chromedp.CSS(`#agent-title`), "Session timer"), chromedp.SendKeys(chromedp.CSS(`#agent-description`), "Keep my session time"),
				chromedp.Focus(chromedp.CSS(`[data-mode="agent"]`)), chromedp.KeyEvent(kb.Tab))
			check(`document.activeElement.dataset.mode === 'paste' && document.activeElement.matches(':focus-visible') && getComputedStyle(document.activeElement).outlineStyle === 'solid'`)
			run(chromedp.KeyEvent(kb.Enter), chromedp.WaitVisible(chromedp.CSS(`.cm-editor`)))
			check(`document.activeElement.dataset.mode === 'paste' && document.querySelector('#snapshot-row').hidden && document.querySelector('#url-field').hidden`)
			run(chromedp.Click(chromedp.CSS(`#ingest-panel .ingest-foot button`)))
			check(`document.querySelector('#status').textContent === 'Paste an artifact first.'`)
			run(chromedp.SendKeys(chromedp.CSS(`#title`), "Saved source"), chromedp.SendKeys(chromedp.CSS(`.cm-content`), "Preserved source"),
				chromedp.Evaluate[chromedp.Void](`window.originalEditor = document.querySelector('.cm-editor')`),
				chromedp.Focus(chromedp.CSS(`[data-mode="paste"]`)), chromedp.KeyEvent(kb.Tab), chromedp.KeyEvent(" "))
			check(`document.activeElement.dataset.mode === 'url' && !document.querySelector('#snapshot-row').hidden && document.querySelector('#source-field').hidden`)
			run(chromedp.Click(chromedp.CSS(`#ingest-panel .ingest-foot button`)))
			check(`document.querySelector('#status').textContent === 'Enter a URL first.'`)
			run(chromedp.SendKeys(chromedp.CSS(`#url-input`), "https://example.test/tool.html"), chromedp.Click(chromedp.CSS(`#snapshot-toggle`)),
				chromedp.Click(chromedp.CSS(`[data-mode="agent"]`)))
			check(`document.querySelector('#agent-title').value === 'Session timer' && document.querySelector('#agent-description').value === 'Keep my session time'`)
			run(chromedp.Click(chromedp.CSS(`[data-mode="paste"]`)))
			check(`document.querySelector('.cm-editor') === originalEditor && document.querySelectorAll('.cm-editor').length === 1 && document.querySelector('#body').value === 'Preserved source' && document.querySelector('#title').value === 'Saved source'`)
			run(chromedp.Click(chromedp.CSS(`#ingest-panel .ingest-foot button`)), chromedp.Poll[chromedp.Void](`document.querySelector('#status').textContent === 'Error: Test ingest refusal'`))
			select {
			case payload := <-payloads:
				require.Equal(t, "Preserved source", payload["body"])
				require.NotContains(t, payload, "snapshot")
				require.NotContains(t, payload, "url")
			case <-ctx.Done():
				t.Fatal("paste did not submit")
			}
			run(chromedp.Click(chromedp.CSS(`[data-mode="url"]`)))
			check(`document.querySelector('#url-input').value === 'https://example.test/tool.html' && !document.querySelector('#snapshot-toggle').checked`)
			run(chromedp.Click(chromedp.CSS(`#ingest-panel .ingest-foot button`)), chromedp.Poll[chromedp.Void](`document.querySelector('#status').textContent === 'Error: Test ingest refusal'`))
			select {
			case payload := <-payloads:
				require.Equal(t, "https://example.test/tool.html", payload["url"])
				require.Equal(t, false, payload["snapshot"])
				require.NotContains(t, payload, "body")
			case <-ctx.Done():
				t.Fatal("URL did not submit")
			}
			run(chromedp.Click(chromedp.CSS(`[data-mode="agent"]`)))
			_, err = chromedp.RunResponse(ctx, chromedp.Click(chromedp.CSS(`#agent-panel button[type="submit"]`)))
			require.NoError(t, err)
			check(`location.pathname === '/agent' && location.search === '' && JSON.parse(sessionStorage.getItem('exhibit:agent-brief')).description === 'Keep my session time' && JSON.parse(sessionStorage.getItem('exhibit:agent-brief')).title === 'Session timer'`)
		})
	}
}
