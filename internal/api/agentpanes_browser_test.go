package api

import (
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
	"github.com/momja/Exhibit/internal/agent"
	"github.com/momja/Exhibit/internal/chromium"
	"github.com/momja/Exhibit/internal/chromium/chromiumtest"
	"github.com/stretchr/testify/require"
)

// Real page/htmx/frame boundaries, without a Pi process or external API key.
func TestAgentPanesBrowser(t *testing.T) {
	ro := newTestRouter(t)
	enablePlatformMode(ro)
	ro.cfg.Agent = &agent.Manager{}
	id := createArtifact(t, ro, map[string]any{
		"title": "A long artifact title for mobile preview",
		"body": `<!doctype html><body style="height:2000px"><h1>Retained preview</h1><script>
		let count = 0;
		addEventListener('message', event => {
		  if (event.data === 'seed-preview') { count++; scrollTo(0, 120); }
		  if (event.data?.readPreview) parent.postMessage({paneTest: true, request: event.data.readPreview, count, scroll: scrollY}, '*');
		});
		</script></body>`,
	})
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
	check := func(expression string) {
		t.Helper()
		ok, err := chromedp.Run(ctx, chromedp.Evaluate[bool](expression))
		require.NoError(t, err)
		require.True(t, ok, expression)
	}
	wait := func(expression string) { t.Helper(); run(chromedp.Poll[chromedp.Void](expression)) }
	capture := func(name string) {
		if dir := os.Getenv("AGENT_PANES_SCREENSHOT_DIR"); dir != "" {
			require.NoError(t, os.MkdirAll(dir, 0755))
			shot, err := chromedp.Run(ctx, chromedp.Screenshot(chromedp.CSS(`body`)))
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(dir, name+".png"), shot, 0644))
		}
	}

	for _, theme := range []string{"light", "dark"} {
		_, err := chromedp.Call(ctx, emulation.SetEmulatedMedia, emulation.SetEmulatedMediaParams{Features: []*emulation.MediaFeature{{Name: "prefers-color-scheme", Value: theme}}})
		require.NoError(t, err)
		for _, width := range []int{320, 375, 390, 430} {
			t.Logf("%s at %d CSS px", theme, width)
			run(chromedp.EmulateViewport(int64(width), 568), chromedp.Navigate(app.URL+"/agent?artifact="+id))
			wait(`typeof window.showPane === 'function'`)
			check(`(() => {
					const row = document.querySelector('.pane-switch').getBoundingClientRect();
					return row.height >= 48 && row.height <= 52 &&
					  [...document.querySelectorAll('.pane-tab')].every(el => el.getBoundingClientRect().height >= 44 && el.getBoundingClientRect().width >= 44) &&
					  document.documentElement.scrollWidth <= innerWidth && document.getElementById('input').getBoundingClientRect().bottom <= innerHeight;
				})()`)
			check(`document.getElementById('pane-chat').getAttribute('role') === 'tabpanel' && document.getElementById('pane-chat').getAttribute('aria-labelledby') === 'tab-chat' && document.getElementById('tab-preview').tabIndex === -1`)
			check(`[...document.querySelectorAll('.pane-tab i')].every(el => el.getAttribute('aria-hidden') === 'true')`)
			capture(fmt.Sprintf("%s-%d-chat", theme, width))
			run(chromedp.Focus(chromedp.CSS(`#tab-chat`)), chromedp.KeyEvent(kb.ArrowRight))
			check(`document.activeElement.id === 'tab-preview' && document.getElementById('tab-preview').getAttribute('aria-selected') === 'true' && document.getElementById('tab-chat').tabIndex === -1 && getComputedStyle(document.getElementById('pane-chat')).visibility === 'hidden' && document.getElementById('pane-chat').inert`)
			run(chromedp.KeyEvent(kb.Tab))
			check(`document.activeElement.id === 'pane-preview'`)
			capture(fmt.Sprintf("%s-%d-preview", theme, width))
			run(chromedp.Focus(chromedp.CSS(`#tab-preview`)), chromedp.KeyEvent(kb.ArrowRight))
			check(`document.activeElement.id === 'tab-chat' && document.getElementById('tab-chat').getAttribute('aria-selected') === 'true'`)
			run(chromedp.KeyEvent(kb.ArrowLeft))
			check(`document.activeElement.id === 'tab-preview'`)
			run(chromedp.KeyEvent(kb.Home))
			check(`document.activeElement.id === 'tab-chat'`)
			run(chromedp.KeyEvent(kb.End))
			check(`document.activeElement.id === 'tab-preview'`)
			run(chromedp.KeyEvent(kb.Home), chromedp.KeyEvent(kb.Tab))
			check(`document.activeElement.id === 'pane-chat'`)
			// The row must grow with large text/long localized labels, never clip.
			run(chromedp.Evaluate[chromedp.Void](`
				  document.documentElement.style.fontSize = '32px';
				  document.querySelector('#tab-chat span').textContent = 'Conversation with the agent';
				  document.querySelector('#tab-preview span').textContent = 'Artifactpreviewwithaverylonglabel';
				`))
			check(`(() => {
				  const row = document.querySelector('.pane-switch').getBoundingClientRect();
				  return row.height > 52 && document.documentElement.scrollWidth <= innerWidth &&
				    [...document.querySelectorAll('.pane-tab span:not(.dot)')].every(el => {
				      const label = el.getBoundingClientRect(), tab = el.parentElement.getBoundingClientRect();
				      return label.left >= tab.left && label.right <= tab.right && label.top >= tab.top && label.bottom <= tab.bottom;
				    }) && document.getElementById('input').getBoundingClientRect().bottom <= innerHeight;
				})()`)
			capture(fmt.Sprintf("%s-%d-large-labels", theme, width))
		}
	}

	// Retain draft, history/message scrollports, and the live iframe across tabs.
	run(chromedp.EmulateViewport(390, 568), chromedp.Navigate(app.URL+"/agent?artifact="+id))
	wait(`typeof window.showPane === 'function'`)
	run(chromedp.Evaluate[chromedp.Void](`
	  window.retainedFrame = document.getElementById('pv-frame');
	  window.addEventListener('message', event => { if (event.source === retainedFrame.contentWindow && event.data?.paneTest) window.previewState = event.data; });
	  document.getElementById('input').value = 'Keep this unsent draft';
	  document.getElementById('messages').textContent = 'A long conversation\n'.repeat(300);
	  document.getElementById('messages').style.whiteSpace = 'pre-wrap';
	  document.getElementById('messages').scrollTop = 120;
	  showPane('preview');
	`))
	wait(`document.getElementById('pv-frame').contentWindow != null`)
	// Wait for the sandboxed document's listener as well as its frame element.
	run(chromedp.Evaluate[chromedp.Void](`window.readPreview = setInterval(() => retainedFrame.contentWindow.postMessage({readPreview: 'initial'}, '*'), 20)`))
	wait(`window.previewState != null`)
	run(chromedp.Evaluate[chromedp.Void](`retainedFrame.contentWindow.postMessage('seed-preview', '*')`))
	wait(`window.previewState.count === 1 && window.previewState.scroll === 120`)
	run(chromedp.Evaluate[chromedp.Void](`clearInterval(window.readPreview)`))
	run(chromedp.Click(chromedp.CSS(`#tab-chat`)))
	check(`document.getElementById('messages').scrollTop === 120 && document.getElementById('input').value === 'Keep this unsent draft'`)
	run(chromedp.Click(chromedp.CSS(`#tab-preview`)),
		chromedp.Evaluate[chromedp.Void](`retainedFrame.contentWindow.postMessage({readPreview: 'after-switch'}, '*')`))
	wait(`window.previewState.request === 'after-switch'`)
	state, err := chromedp.Run(ctx, chromedp.Evaluate[struct {
		Count  int `json:"count"`
		Scroll int `json:"scroll"`
	}](`window.previewState`))
	require.NoError(t, err)
	require.Equal(t, 1, state.Count, "the live document must not reload")
	require.Equal(t, 120, state.Scroll, "the preview must retain its scroll position")
	check(`document.getElementById('pv-frame') === window.retainedFrame`)
	run(chromedp.Evaluate[chromedp.Void](`showPane('chat'); document.getElementById('input').focus()`))
	// Programmatic links/nudges move focus out of the pane they hide.
	run(chromedp.Evaluate[chromedp.Void](`showPane('preview')`))
	check(`document.activeElement.id === 'tab-preview'`)

	// Same-pane actions keep their focus so Enter can toggle snippet mode off.
	run(chromedp.Focus(chromedp.CSS(`#snip-btn`)), chromedp.KeyEvent(kb.Enter))
	check(`document.activeElement.id === 'snip-btn' && document.getElementById('snip-btn').classList.contains('active')`)
	run(chromedp.KeyEvent(kb.Enter))
	check(`document.activeElement.id === 'snip-btn' && !document.getElementById('snip-btn').classList.contains('active')`)

	// Actual htmx content replacement must keep the tab/panel association.
	run(chromedp.Evaluate[chromedp.Void](`document.body.dispatchEvent(new CustomEvent('exhibit:artifact-saved'))`))
	wait(`document.getElementById('pv-frame') !== window.retainedFrame`)
	check(`document.getElementById('pane-preview').getAttribute('aria-labelledby') === 'tab-preview' && document.getElementById('tab-preview').getAttribute('aria-selected') === 'true'`)

	// Desktop keeps two visible regions; resize never leaves focus hidden.
	run(chromedp.Focus(chromedp.CSS(`#tab-preview`)), chromedp.EmulateViewport(1280, 800))
	wait(`document.getElementById('pane-preview').getAttribute('role') === 'region'`)
	check(`document.activeElement.id === 'pane-preview' && !document.getElementById('pane-chat').inert && !document.getElementById('pane-preview').inert && getComputedStyle(document.getElementById('pane-chat')).display !== 'none' && getComputedStyle(document.getElementById('pane-preview')).display !== 'none' && getComputedStyle(document.querySelector('.pane-switch')).display === 'none'`)
	run(chromedp.Focus(chromedp.CSS(`#input`)), chromedp.Evaluate[chromedp.Void](`showPane('preview')`))
	check(`document.activeElement.id === 'input'`)
	capture("desktop")
	run(chromedp.EmulateViewport(390, 568))
	wait(`document.activeElement.id === 'tab-preview'`)

	// An empty preview still offers a Tab destination and a route back to Chat.
	run(chromedp.Navigate(app.URL + "/agent"))
	wait(`typeof window.showPane === 'function'`)
	run(chromedp.Click(chromedp.CSS(`#tab-preview`)), chromedp.KeyEvent(kb.Tab))
	check(`document.activeElement.id === 'pane-preview' && document.getElementById('empty-preview') != null`)
}

// Let the browser render the new viewport before delivering the pane handler.
// This deterministically exposes CSS hiding a focused control before the
// handler can capture it, rather than depending on machine load/event timing.
func TestAgentPaneBreakpointFocusBrowser(t *testing.T) {
	ro := newTestRouter(t)
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
	ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	_, err = chromedp.Call(ctx, page.AddScriptToEvaluateOnNewDocument, page.AddScriptToEvaluateOnNewDocumentParams{Source: `(() => {
	  const matchMedia = window.matchMedia.bind(window);
	  window.matchMedia = query => {
	    const media = matchMedia(query);
	    if (query === '(max-width:640px)') {
	      const add = media.addEventListener.bind(media);
	      media.addEventListener = (type, listener, options) => add(type, event => {
	        window.pendingPaneChange = () => listener.call(media, event);
	      }, options);
	    }
	    return media;
	  };
	})()`})
	require.NoError(t, err)
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
	resizeBeforeHandler := func(width int64) {
		t.Helper()
		run(chromedp.EmulateViewport(width, 568),
			chromedp.Poll[chromedp.Void](`typeof window.pendingPaneChange === 'function'`),
			chromedp.Evaluate[chromedp.Void](`new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)))`, chromedp.EvalAwaitPromise))
	}
	flushHandler := func() {
		t.Helper()
		run(chromedp.Evaluate[chromedp.Void](`window.pendingPaneChange(); window.pendingPaneChange = null`))
	}
	run(chromedp.EmulateViewport(1280, 800), chromedp.Navigate(app.URL+"/agent"),
		chromedp.Poll[chromedp.Void](`typeof window.showPane === 'function'`),
		chromedp.Evaluate[chromedp.Void](`showPane('preview')`), chromedp.Focus(chromedp.CSS(`#input`)))

	// Desktop -> mobile: keep the input focused until its hide is handled.
	resizeBeforeHandler(390)
	check(`document.activeElement.id === 'input'`)
	flushHandler()
	check(`document.activeElement.id === 'tab-preview' && document.getElementById('pane-chat').inert && document.getElementById('pane-preview').getAttribute('role') === 'tabpanel'`)

	// Mobile -> desktop: keep the selected tab focused until its hide is handled.
	resizeBeforeHandler(1280)
	check(`document.activeElement.id === 'tab-preview'`)
	flushHandler()
	check(`document.activeElement.id === 'pane-preview' && !document.getElementById('pane-chat').inert && !document.getElementById('pane-preview').inert && document.getElementById('pane-preview').getAttribute('role') === 'region'`)

	// A deliberate focus change during the delay must win over the old input.
	run(chromedp.Focus(chromedp.CSS(`#input`)))
	resizeBeforeHandler(390)
	run(chromedp.Focus(chromedp.CSS(`.app-header a`)))
	flushHandler()
	check(`document.activeElement.matches('.app-header a')`)

	// The desktop divider also disappears at this breakpoint.
	resizeBeforeHandler(1280)
	flushHandler()
	run(chromedp.Focus(chromedp.CSS(`#agent-pane-resizer`)))
	resizeBeforeHandler(390)
	check(`document.activeElement.id === 'agent-pane-resizer'`)
	flushHandler()
	check(`document.activeElement.id === 'tab-preview'`)
}
