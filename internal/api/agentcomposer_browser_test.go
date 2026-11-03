package api

import (
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
	"github.com/momja/Exhibit/internal/chromium"
	"github.com/momja/Exhibit/internal/chromium/chromiumtest"
	"github.com/stretchr/testify/require"
)

// Exercise the actual composer while a controllable session stream displays
// Stop. Its empty placeholder must fit one line without changing how messages
// wrap or how Shift+Enter inserts a new line.
func TestAgentComposerBrowser(t *testing.T) {
	browser, err := chromium.New(chromiumtest.Start(t))
	require.NoError(t, err)
	ctx, cancel, err := browser.Tab(context.Background())
	require.NoError(t, err)
	ctx, timeout := context.WithTimeout(ctx, 60*time.Second)
	defer timeout()
	handler, id, events := markdownBrowserFixture(t)
	app := httptest.NewServer(handler)
	defer func() {
		cancel()
		app.CloseClientConnections()
		app.Close()
	}()
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

	for _, mode := range []string{"build", "modify"} {
		path := "/agent"
		if mode == "modify" {
			path += "?artifact=" + id
		}
		run(chromedp.EmulateViewport(1280, 900), chromedp.Navigate(app.URL+path),
			chromedp.Poll[chromedp.Void](`typeof window.send === 'function'`),
			chromedp.SendKeys(chromedp.CSS(`#input`), "Start a sample task"),
			chromedp.Click(chromedp.CSS(`#send-btn`)))
		events <- []byte(`{"type":"agent_start"}`)
		run(chromedp.Poll[chromedp.Void](`getComputedStyle(document.querySelector('#stop-btn')).display !== 'none' && document.querySelector('#input').value === ''`))
		for _, scenario := range []struct {
			name  string
			width int64
		}{
			{"phone-320", 320},
			{"phone-390", 390},
			{"desktop-narrow", 1280},
		} {
			for _, fontSize := range []int{16, 24, 32} {
				t.Run(fmt.Sprintf("%s/%s/font-%d", mode, scenario.name, fontSize), func(t *testing.T) {
					run(chromedp.EmulateViewport(scenario.width, 900))
					if scenario.width > 640 {
						run(chromedp.Focus(chromedp.CSS(`#agent-pane-resizer`)), chromedp.KeyEvent(kb.Home),
							chromedp.Poll[chromedp.Void](`document.querySelector('#pane-chat').getBoundingClientRect().width === 280`))
					}
					run(chromedp.Evaluate[chromedp.Void](fmt.Sprintf(`document.querySelector('#input').style.fontSize = '%dpx'`, fontSize)))
					metrics, err := chromedp.Run(ctx, chromedp.Evaluate[struct {
						ScrollHeight float64 `json:"scrollHeight"`
						ClientHeight float64 `json:"clientHeight"`
						SingleLine   float64 `json:"singleLine"`
						Fits         bool    `json:"fits"`
						Placeholder  string  `json:"placeholder"`
					}](`(() => {
						const input = document.querySelector('#input');
						const style = getComputedStyle(input);
						const row = document.querySelector('.compose-row').getBoundingClientRect();
						return {
							scrollHeight: input.scrollHeight,
							clientHeight: input.clientHeight,
							singleLine: parseFloat(style.lineHeight) + parseFloat(style.paddingTop) + parseFloat(style.paddingBottom),
							placeholder: input.placeholder,
							fits: [...document.querySelector('.compose-row').children].every(control => {
								const bounds = control.getBoundingClientRect();
								return bounds.left >= row.left && bounds.right <= row.right + 1;
							}) && document.documentElement.scrollWidth <= innerWidth
						};
					})()`))
					require.NoError(t, err)
					require.LessOrEqual(t, metrics.ScrollHeight, metrics.SingleLine+1, "empty placeholder must occupy at most one line: %+v", metrics)
					require.LessOrEqual(t, metrics.ScrollHeight, metrics.ClientHeight+1, "empty placeholder must not overflow vertically: %+v", metrics)
					require.True(t, metrics.Fits, "composer controls must fit the pane")
					require.Equal(t, "Ask Exhibit...", metrics.Placeholder)
					if fontSize == 32 {
						if root := os.Getenv("AGENT_COMPOSER_SCREENSHOT_DIR"); root != "" {
							require.NoError(t, os.MkdirAll(root, 0755))
							shot, err := chromedp.Run(ctx, chromedp.Screenshot(chromedp.CSS(`#pane-chat`)))
							require.NoError(t, err)
							require.NoError(t, os.WriteFile(filepath.Join(root, mode+"-"+scenario.name+".png"), shot, 0644))
						}
					}
				})
			}
		}
		// The same narrow field must still grow for wrapped and explicit lines.
		run(chromedp.Evaluate[chromedp.Void](`window.emptyComposerHeight=document.querySelector('#input').getBoundingClientRect().height`),
			chromedp.SendKeys(chromedp.CSS(`#input`), "A message long enough to wrap over several lines inside the narrow chat pane"))
		check(`document.querySelector('#input').getBoundingClientRect().height > window.emptyComposerHeight`)
		check(`document.querySelector('#input').scrollWidth <= document.querySelector('#input').clientWidth + 1`)
		run(chromedp.KeyEvent(kb.End), chromedp.KeyEvent(kb.Shift+kb.Enter))
		check(`document.querySelector('#input').value.endsWith('\n')`)
		run(chromedp.Evaluate[chromedp.Void](`document.querySelector('#input').value=''; document.querySelector('#input').dispatchEvent(new Event('input',{bubbles:true}))`))
		events <- []byte(`{"type":"exhibit_questions","requestId":"composer-question","pending":true}`)
		run(chromedp.Poll[chromedp.Void](`document.querySelector('#hint').hidden`))
		check(`document.querySelector('#input').placeholder === 'Ask Exhibit...'`)
		events <- []byte(`{"type":"agent_settled"}`)
		run(chromedp.Poll[chromedp.Void](`getComputedStyle(document.querySelector('#stop-btn')).display === 'none'`))
		check(`document.querySelector('#input').placeholder === 'Ask Exhibit...'`)
	}
}
