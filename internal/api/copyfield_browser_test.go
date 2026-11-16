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
	run(chromedp.Navigate(srv.URL), chromedp.Poll[chromedp.Void](`window.application?.controllers.length === 3`))
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
	run(chromedp.Evaluate[chromedp.Void](`const field=document.querySelector('.copy-field');field.outerHTML=field.outerHTML;`), chromedp.Poll[chromedp.Void](`window.application.controllers.length === 3`))
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

// Exercise consumers in their actual pages: token values are browser-only,
// and sharing fields reconnect after the real htmx link replacement.
func TestCopyFieldTokenBrowser(t *testing.T) {
	ro := newTestRouter(t)
	ro.cfg.MCPURL = "https://agents.example.test/mcp"
	app := httptest.NewServer(ro)
	defer app.Close()
	b := newModalBrowser(t, 390)
	b.run(chromedp.Navigate(app.URL + "/settings"))
	b.wait(`document.getElementById('agent-grant-form') && window.apiFetch`)
	b.eval(`window.copied=[];Object.defineProperty(navigator,'clipboard',{configurable:true,value:{writeText:async text=>copied.push(text)}});document.getElementById('agent-grant-label').value='Copy test';document.getElementById('agent-grant-form').requestSubmit()`)
	b.wait(`!document.getElementById('agent-grant-issued').hidden && !document.getElementById('agent-grant-issue').disabled`)
	b.run(chromedp.Click(chromedp.CSS(`#agent-grant-token-copy i`)))
	b.wait(`copied.length===1`)
	b.check(`copied[0] === document.getElementById('agent-grant-token').value && copied[0].length>0`)
	b.eval(`document.getElementById('agent-grant-dismiss').click()`)
	b.check(`document.getElementById('agent-grant-token').value==='' && document.getElementById('agent-grant-issued').hidden && document.getElementById('agent-grant-remote-command').textContent==='' && document.getElementById('agent-grant-remote-config').textContent===''`)
}

func TestShareCopyFieldsBrowser(t *testing.T) {
	for _, width := range []int64{1280, 390, 320} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			ro := newTestRouter(t)
			id := createTestArtifact(t, ro, "Copy fields")
			app := httptest.NewServer(ro)
			defer app.Close()
			ro.cfg.AppOrigin = app.URL
			b := newModalBrowser(t, width)
			b.run(chromedp.Navigate(app.URL + "/artifacts/" + id))
			b.wait(`document.getElementById('share-open') && window.apiFetch`)
			b.eval(`window.copied=[];Object.defineProperty(navigator,'clipboard',{configurable:true,value:{writeText:async text=>copied.push(text)}});document.getElementById('share-open').click();document.getElementById('share-link-toggle').click()`)
			b.wait(`document.getElementById('share-link-url-copy') && !document.getElementById('share-close').disabled`)
			b.run(chromedp.Click(chromedp.CSS(`#share-link-url-copy i`)))
			b.wait(`copied.length===1`)
			b.check(`copied[0]===document.getElementById('share-link-url').value && document.getElementById('share-link-url').labels[0].textContent==='Public link'`)
			b.eval(`document.getElementById('share-widget-details').open=true`)
			b.run(chromedp.Click(chromedp.CSS(`#share-widget-embed-copy i`)))
			b.wait(`copied.length===2`)
			b.check(`copied[1]===document.getElementById('share-widget-embed').value && copied[1].startsWith('<iframe ')`)
			b.eval(`document.getElementById('share-link-replace').click()`)
			b.wait(`document.getElementById('share-link-url').value!==copied[0] && !document.getElementById('share-close').disabled`)
			b.run(chromedp.Click(chromedp.CSS(`#share-link-url-copy`)))
			b.wait(`copied.length===3`)
			b.check(`copied[2]===document.getElementById('share-link-url').value && document.getElementById('share-widget-details').open`)
			// Native fieldset locking continues to cover the generic controls.
			b.eval(`document.getElementById('share-body').disabled=true;document.getElementById('share-link-url-copy').click()`)
			b.check(`copied.length===3 && document.getElementById('share-link-url-copy').matches(':disabled')`)
			b.eval(`document.getElementById('share-body').disabled=false;Object.defineProperty(navigator,'clipboard',{configurable:true,value:undefined})`)
			b.run(chromedp.Click(chromedp.CSS(`#share-widget-embed-copy`)))
			b.wait(`document.getElementById('share-widget-embed-status').textContent.includes('manually')`)
			b.check(`document.activeElement.id==='share-widget-embed' && document.activeElement.selectionEnd===document.activeElement.value.length`)
			b.eval(`Object.defineProperty(navigator,'clipboard',{configurable:true,value:{writeText:async text=>copied.push(text)}});document.getElementById('share-widget-embed-copy').click()`)
			b.wait(`document.getElementById('share-widget-embed-status').textContent==='' && document.getElementById('share-link-url-status').textContent===''`)
			b.eval(`document.getElementById('share-close').focus();document.querySelectorAll('.copy-field-input').forEach(input=>{input.scrollLeft=0})`)

			for _, scheme := range []string{"light", "dark"} {
				_, err := chromedp.Call(b.ctx, emulation.SetEmulatedMedia, emulation.SetEmulatedMediaParams{Features: []*emulation.MediaFeature{{Name: "prefers-color-scheme", Value: scheme}}})
				require.NoError(t, err)
				b.eval(`document.querySelector('.modal-body').scrollTop=0`)
				b.check(`document.getElementById('share-panel').scrollWidth===document.getElementById('share-panel').clientWidth`)
				b.capture("Exh-qq4y/sharing-"+scheme, width)
			}
		})
	}
}
