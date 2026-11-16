package api

import (
	"context"
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

func TestCopyFieldBrowser(t *testing.T) {
	browser, err := chromium.New(chromiumtest.Start(t))
	require.NoError(t, err)
	ctx, cancel, err := browser.Tab(context.Background())
	require.NoError(t, err)
	defer cancel()
	ctx, cancel = context.WithTimeout(ctx, 40*time.Second)
	defer cancel()
	ro := newTestRouter(t)
	ro.cfg.MCPURL = "https://exhibit-mcp.fly.dev/mcp?literal='\"<&"
	mux := http.NewServeMux()
	mux.Handle("/assets/", assetsHandler())
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		view, err := ro.agentGrantsPanel(r)
		require.NoError(t, err)
		panel, err := renderPage("agentGrantsPanel", view)
		require.NoError(t, err)
		fmt.Fprint(w, `<!doctype html><html><head><meta name="viewport" content="width=device-width,initial-scale=1"><link rel="stylesheet" href="/assets/gallery/tokens.css"><link rel="stylesheet" href="/assets/gallery/components.css"><link rel="stylesheet" href="/assets/gallery/settings.css"><link rel="stylesheet" href="/assets/phosphor/regular.css"></head><body><main style="max-width:760px;margin:24px auto;padding:16px">`, panel, `</main><script type="module">import { application } from '/assets/gallery/controllers.js'; window.application=application;</script></body></html>`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	run := func(actions ...chromedp.Action[chromedp.Void]) {
		t.Helper()
		require.NoError(t, chromedp.Do(ctx, actions...))
	}
	check := func(expression string) {
		t.Helper()
		ok, err := chromedp.Run(ctx, chromedp.Evaluate[bool](expression))
		require.NoError(t, err)
		require.True(t, ok, expression)
	}
	run(chromedp.Navigate(srv.URL), chromedp.Poll[chromedp.Void](`window.application?.controllers.length === 2`))
	run(chromedp.Evaluate[chromedp.Void](`window.copied=[];Object.defineProperty(navigator,'clipboard',{configurable:true,value:{writeText:async text=>copied.push(text)}})`))
	check(`document.querySelector('#agent-mcp-url').value === "https://exhibit-mcp.fly.dev/mcp?literal='\"<&"`)
	// Native keyboard activation copies literal bytes from the focused instance.
	run(chromedp.Focus(chromedp.CSS(`[aria-label="Copy URL"]`)), chromedp.KeyEvent(kb.Enter), chromedp.Poll[chromedp.Void](`copied.length === 1`))
	check(`copied[0] === document.querySelector('#agent-mcp-url').value && document.querySelector('#agent-mcp-url-status').textContent === 'Copied.'`)
	check(`document.querySelector('#agent-mcp-command-status').textContent === ''`)
	run(chromedp.Click(chromedp.CSS(`[aria-label="Copy command"]`)), chromedp.Poll[chromedp.Void](`copied.length === 2`))
	check(`copied[1] === document.querySelector('#agent-mcp-command').value`)
	// Rejection and missing API both keep a manual selection available.
	for _, clipboard := range []string{`{writeText:async()=>{throw Error('denied')}}`, `undefined`} {
		run(chromedp.Evaluate[chromedp.Void](`Object.defineProperty(navigator,'clipboard',{configurable:true,value:`+clipboard+`})`), chromedp.Click(chromedp.CSS(`[aria-label="Copy URL"]`)), chromedp.Poll[chromedp.Void](`document.querySelector('#agent-mcp-url-status').textContent.includes('manually')`))
		check(`document.activeElement.id === 'agent-mcp-url' && document.activeElement.selectionEnd === document.activeElement.value.length`)
	}
	// A replacement from server markup connects independently, with no page wiring.
	run(chromedp.Evaluate[chromedp.Void](`const field=document.querySelector('.copy-field');field.outerHTML=field.outerHTML;`), chromedp.Poll[chromedp.Void](`window.application.controllers.length === 2`))
	run(chromedp.Evaluate[chromedp.Void](`Object.defineProperty(navigator,'clipboard',{configurable:true,value:{writeText:async text=>copied.push(text)}})`), chromedp.Click(chromedp.CSS(`[aria-label="Copy URL"]`)), chromedp.Poll[chromedp.Void](`copied.length === 3`))
	run(chromedp.Poll[chromedp.Void](`document.querySelector('#agent-mcp-url-status').textContent === ''`))
	for _, width := range []int64{1280, 390, 320} {
		for _, scheme := range []string{"light", "dark"} {
			_, err = chromedp.Call(ctx, emulation.SetDeviceMetricsOverride, emulation.SetDeviceMetricsOverrideParams{Width: width, Height: 1000, DeviceScaleFactor: 1})
			require.NoError(t, err)
			_, err = chromedp.Call(ctx, emulation.SetEmulatedMedia, emulation.SetEmulatedMediaParams{Features: []*emulation.MediaFeature{{Name: "prefers-color-scheme", Value: scheme}}})
			require.NoError(t, err)
			check(`document.documentElement.scrollWidth <= innerWidth`)
			check(`Array.from(document.querySelectorAll('.copy-field')).every(field=>{const input=field.querySelector('input').getBoundingClientRect(), button=field.querySelector('button').getBoundingClientRect();return button.left>=input.left && button.right<=input.right && button.top>=input.top && button.bottom<=input.bottom})`)
			if dir := os.Getenv("COPY_FIELD_SCREENSHOT_DIR"); dir != "" {
				require.NoError(t, os.MkdirAll(dir, 0755))
				shot, err := chromedp.Run(ctx, chromedp.FullScreenshot(90))
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(filepath.Join(dir, fmt.Sprintf("%d-%s.png", width, scheme)), shot, 0644))
			}
		}
	}
}
