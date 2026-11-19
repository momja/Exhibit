package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/momja/Exhibit/internal/chromium"
	"github.com/momja/Exhibit/internal/chromium/chromiumtest"
	"github.com/stretchr/testify/require"
)

// Exercise real SSE handlers and htmx swaps: a stable src alone is insufficient
// if the containing swap still replaces the artifact iframe.
func TestAgentPreviewStampsMoveIndependentlyBrowser(t *testing.T) {
	for _, bound := range []bool{true, false} {
		t.Run(fmt.Sprintf("bound=%t", bound), func(t *testing.T) { testAgentPreviewSaves(t, bound) })
	}
}

func testAgentPreviewSaves(t *testing.T, bound bool) {
	browser, err := chromium.New(chromiumtest.Start(t))
	require.NoError(t, err)
	ctx, cancel, err := browser.Tab(context.Background())
	require.NoError(t, err)
	defer cancel()
	ctx, cancel = context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	handler, id, events := markdownBrowserFixture(t)
	// Hold one preview response to exercise overlapping widget/artifact saves.
	var holdNext atomic.Bool
	held, release := make(chan struct{}, 1), make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/partials/agent-preview" && holdNext.CompareAndSwap(true, false) {
			held <- struct{}{}
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
		}
		handler.ServeHTTP(w, r)
	}))
	defer func() { cancel(); srv.CloseClientConnections(); srv.Close() }()
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
	remember := func() {
		run(chromedp.Evaluate[chromedp.Void](`window.oldFrame = document.getElementById('pv-frame'); window.oldFrameSrc = oldFrame.src; window.oldWidget = document.querySelector('.pv-widget iframe') || document.getElementById('pv-widget'); window.oldWidgetSrc = oldWidget.src || '';`))
	}
	emit := func(kind string) {
		events <- []byte(fmt.Sprintf(`{"type":%q,"artifactId":%q,"title":"Reading list","action":"updated"}`, kind, id))
	}
	pageURL := srv.URL + "/agent"
	if bound {
		pageURL += "?artifact=" + id
	}
	run(chromedp.Navigate(pageURL),
		chromedp.Poll[chromedp.Void](`typeof window.send === 'function'`),
		chromedp.SendKeys(chromedp.CSS("#input"), "Refresh preview"),
		chromedp.Click(chromedp.CSS("#send-btn")))
	events <- []byte(`{"type":"agent_start"}`)
	if bound {
		run(chromedp.Poll[chromedp.Void](`document.getElementById('pv-widget') !== null`))
		check(`document.getElementById('pv-frame') !== null && !document.querySelector('.pv-widget iframe')`)
		remember()
	} else {
		check(`document.getElementById('empty-preview') !== null && !document.getElementById('pv-frame')`)
	}
	payload, err := json.Marshal(map[string]string{"body": "<b>Widget</b>"})
	require.NoError(t, err)
	req := httptest.NewRequest("PUT", "/api/artifacts/"+id+"/widget", strings.NewReader(string(payload)))
	req.Header.Set("Authorization", authHeader())
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	emit("exhibit_widget_saved")
	run(chromedp.Poll[chromedp.Void](`document.querySelector('.pv-widget iframe') !== null`))
	if bound {
		check(`document.getElementById('pv-frame') === oldFrame && oldFrame.src === oldFrameSrc`)
	}
	remember()
	check(`new URL(oldFrameSrc).searchParams.get('r') !== new URL(oldWidgetSrc).searchParams.get('r')`)
	for i := 0; i < 2; i++ {
		emit("exhibit_widget_saved")
		run(chromedp.Poll[chromedp.Void](`document.querySelector('.pv-widget iframe') && document.querySelector('.pv-widget iframe') !== oldWidget`))
		check(`document.getElementById('pv-frame') === oldFrame && oldFrame.src === oldFrameSrc`)
		check(`new URL(document.querySelector('.pv-widget iframe').src).searchParams.get('r') !== new URL(oldWidgetSrc).searchParams.get('r')`)
		remember()
	}
	for _, kind := range []string{"exhibit_artifact_saved", "exhibit_state_changed"} {
		emit(kind)
		run(chromedp.Poll[chromedp.Void](`document.getElementById('pv-frame') && document.getElementById('pv-frame') !== oldFrame && document.getElementById('pv-frame').src !== oldFrameSrc`))
		check(`new URL(document.getElementById('pv-frame').src).searchParams.get('r') !== new URL(oldFrameSrc).searchParams.get('r')`)
		remember()
	}
	// A queued widget refresh must never displace a pending artifact refresh.
	remember()
	holdNext.Store(true)
	emit("exhibit_widget_saved")
	select {
	case <-held:
	case <-ctx.Done():
		t.Fatal("widget request did not start")
	}
	run(chromedp.Evaluate[chromedp.Void](`window.messageCount = document.querySelectorAll('.msg').length;`))
	emit("exhibit_artifact_saved")
	emit("exhibit_widget_saved")
	run(chromedp.Poll[chromedp.Void](`document.querySelectorAll('.msg').length >= messageCount + 2`))
	close(release)
	run(chromedp.Poll[chromedp.Void](`document.getElementById('pv-frame') && document.getElementById('pv-frame').src !== oldFrameSrc`))
	// Use the actual server-rendered historical viewer, then await the SSE
	// notification to ensure widget saves were processed without displacing it.
	run(chromedp.Evaluate[chromedp.Void](fmt.Sprintf(`htmx.ajax('GET', '/partials/version-viewer?artifact=%s&seq=1', {target:'#pane-preview', swap:'innerHTML'});`, id)),
		chromedp.Poll[chromedp.Void](`document.getElementById('version-viewer') !== null`),
		chromedp.Evaluate[chromedp.Void](`window.oldViewer = document.getElementById('version-viewer'); window.messageCount = document.querySelectorAll('.msg').length; window.previewRefreshes = 0; for (const name of ['exhibit:artifact-saved', 'exhibit:widget-saved']) document.body.addEventListener(name, () => window.previewRefreshes++);`))
	emit("exhibit_widget_saved")
	run(chromedp.Poll[chromedp.Void](`document.querySelectorAll('.msg').length > messageCount`))
	check(`previewRefreshes === 0 && document.getElementById('version-viewer') === oldViewer && !document.getElementById('pv-frame')`)
	emit("exhibit_artifact_saved")
	run(chromedp.Poll[chromedp.Void](`document.getElementById('pv-frame') !== null && !document.getElementById('version-viewer')`))
	if dir := os.Getenv("AGENT_PREVIEW_SCREENSHOT_DIR"); dir != "" {
		require.NoError(t, os.MkdirAll(dir, 0755))
		shot, err := chromedp.Run(ctx, chromedp.Screenshot(chromedp.CSS("body")))
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(dir, fmt.Sprintf("preview-bound-%t.png", bound)), shot, 0644))
	}
}
