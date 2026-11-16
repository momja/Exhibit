package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/momja/Exhibit/internal/chromium"
	"github.com/momja/Exhibit/internal/chromium/chromiumtest"
	"github.com/stretchr/testify/require"
)

// Bursts retain one queued refresh. A fence on the same htmx queue makes
// request counts deterministic without a sleep or an idle-time assumption.
func TestAgentPreviewSaveBurstsCoalesceBrowser(t *testing.T) {
	for _, kind := range []string{"exhibit_artifact_saved", "exhibit_widget_saved"} {
		t.Run(kind, func(t *testing.T) {
			browser, err := chromium.New(chromiumtest.Start(t))
			require.NoError(t, err)
			ctx, cancel, err := browser.Tab(context.Background())
			require.NoError(t, err)
			defer cancel()
			ctx, cancel = context.WithTimeout(ctx, 60*time.Second)
			defer cancel()
			handler, id, events := markdownBrowserFixture(t)
			var holdNext atomic.Bool
			var requests atomic.Int64
			held, release := make(chan struct{}, 1), make(chan struct{})
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/preview-burst-fence" {
					w.WriteHeader(http.StatusNoContent)
					return
				}
				if r.URL.Path == "/partials/agent-preview" {
					requests.Add(1)
				}
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
			run := func(actions ...chromedp.Action[chromedp.Void]) { require.NoError(t, chromedp.Do(ctx, actions...)) }
			emit := func() {
				events <- []byte(fmt.Sprintf(`{"type":%q,"artifactId":%q,"title":"Reading list","action":"updated"}`, kind, id))
			}
			run(chromedp.Navigate(srv.URL+"/agent?artifact="+id),
				chromedp.Poll[chromedp.Void](`typeof window.send === 'function'`),
				chromedp.SendKeys(chromedp.CSS("#input"), "Refresh preview"),
				chromedp.Click(chromedp.CSS("#send-btn")))
			events <- []byte(`{"type":"agent_start"}`)
			run(chromedp.Poll[chromedp.Void](`document.getElementById('pv-frame') !== null`))
			requests.Store(0)
			run(chromedp.Evaluate[chromedp.Void](`window.fenceDone = false; const fence = document.createElement('div'); fence.hidden = true; fence.setAttribute('hx-get', '/preview-burst-fence'); fence.setAttribute('hx-trigger', 'preview:fence'); fence.setAttribute('hx-sync', '#pane-preview:queue all'); fence.setAttribute('hx-swap', 'none'); fence.addEventListener('htmx:finally:request', () => window.fenceDone = true); document.body.append(fence); htmx.process(fence); window.queueFence = () => fence.dispatchEvent(new CustomEvent('preview:fence'));`))
			holdNext.Store(true)
			emit()
			select {
			case <-held:
			case <-ctx.Done():
				t.Fatal("preview request did not start")
			}
			run(chromedp.Evaluate[chromedp.Void](`window.mc = document.querySelectorAll('.msg').length;`))
			for i := 0; i < 3; i++ {
				emit()
			}
			run(chromedp.Poll[chromedp.Void](`document.querySelectorAll('.msg').length >= mc + 3`))
			run(chromedp.Evaluate[chromedp.Void](`queueFence()`))
			close(release)
			run(chromedp.Poll[chromedp.Void](`fenceDone`))
			require.Equal(t, int64(2), requests.Load(), "one in-flight refresh plus one coalesced refresh")
		})
	}
}
