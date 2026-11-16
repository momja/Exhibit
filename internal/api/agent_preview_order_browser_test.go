package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/momja/Exhibit/internal/chromium"
	"github.com/momja/Exhibit/internal/chromium/chromiumtest"
	"github.com/stretchr/testify/require"
)

// A pane response rendered before a widget save must not overwrite its tile
// or detach the widget refresh target when delivered after the save.
func TestAgentPreviewStalePaneBeforeWidgetSaveBrowser(t *testing.T) {
	browser, err := chromium.New(chromiumtest.Start(t))
	require.NoError(t, err)
	ctx, cancel, err := browser.Tab(context.Background())
	require.NoError(t, err)
	defer cancel()
	ctx, cancel = context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	handler, id, events := markdownBrowserFixture(t)
	var holdAfterRender atomic.Bool
	held, release := make(chan struct{}, 1), make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/partials/agent-preview" && holdAfterRender.CompareAndSwap(true, false) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, r) // render now, with pre-widget state
			held <- struct{}{}
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
			for k, v := range rec.Header() {
				w.Header()[k] = v
			}
			w.WriteHeader(rec.Code)
			_, _ = w.Write(rec.Body.Bytes())
			return
		}
		handler.ServeHTTP(w, r)
	}))
	defer func() { cancel(); srv.CloseClientConnections(); srv.Close() }()
	run := func(actions ...chromedp.Action[chromedp.Void]) {
		t.Helper()
		require.NoError(t, chromedp.Do(ctx, actions...))
	}
	emit := func(kind string) {
		events <- []byte(fmt.Sprintf(`{"type":%q,"artifactId":%q,"title":"Reading list","action":"updated"}`, kind, id))
	}
	run(chromedp.Navigate(srv.URL+"/agent?artifact="+id),
		chromedp.Poll[chromedp.Void](`typeof window.send === 'function'`),
		chromedp.SendKeys(chromedp.CSS("#input"), "Refresh preview"),
		chromedp.Click(chromedp.CSS("#send-btn")))
	events <- []byte(`{"type":"agent_start"}`)
	run(chromedp.Poll[chromedp.Void](`document.getElementById('pv-widget') !== null`))

	run(chromedp.Evaluate[chromedp.Void](`window.oldFrame = document.getElementById('pv-frame'); window.messageCount = document.querySelectorAll('.msg').length; window.refreshOrder = []; document.getElementById('pv-widget-refresh').addEventListener('htmx:before:request', () => refreshOrder.push('widget')); document.getElementById('pane-preview').addEventListener('htmx:after:swap', () => refreshOrder.push('pane'));`))
	holdAfterRender.Store(true)
	emit("exhibit_artifact_saved")
	select {
	case <-held:
	case <-ctx.Done():
		t.Fatal("pane request did not start")
	}
	payload, err := json.Marshal(map[string]string{"body": "<b>Widget</b>"})
	require.NoError(t, err)
	req := httptest.NewRequest("PUT", "/api/artifacts/"+id+"/widget", strings.NewReader(string(payload)))
	req.Header.Set("Authorization", authHeader())
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	emit("exhibit_widget_saved")
	run(chromedp.Poll[chromedp.Void](`document.querySelectorAll('.msg').length >= messageCount + 2`))
	ordered, err := chromedp.Run(ctx, chromedp.Evaluate[bool](`refreshOrder.length === 0`))
	require.NoError(t, err)
	require.True(t, ordered, "widget request must remain queued until the held pane swaps")
	close(release)
	run(chromedp.Poll[chromedp.Void](`document.getElementById('pv-frame') && document.getElementById('pv-frame') !== oldFrame`),
		chromedp.Poll[chromedp.Void](`document.querySelector('.pv-widget iframe') !== null`))
	ordered, err = chromedp.Run(ctx, chromedp.Evaluate[bool](`refreshOrder.join(',') === 'pane,widget'`))
	require.NoError(t, err)
	require.True(t, ordered, "widget request must start after the pane swap")
	ok, err := chromedp.Run(ctx, chromedp.Evaluate[bool](`document.querySelector('.pv-widget iframe') !== null`))
	require.NoError(t, err)
	require.True(t, ok, "stale pre-save pane response overwrote the fresh widget tile")
}
