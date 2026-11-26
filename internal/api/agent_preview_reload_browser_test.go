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
	"github.com/momja/Exhibit/internal/agent"
	"github.com/momja/Exhibit/internal/chromium"
	"github.com/momja/Exhibit/internal/chromium/chromiumtest"
	"github.com/stretchr/testify/require"
)

// Keep the two entry points independent so a broken reload cannot hide a
// broken swap. Only the agent subprocess is replaced by a controlled stream.
func TestAgentPreviewReloadAndEditBrowser(t *testing.T) {
	for _, trigger := range []string{"reload after creation", "relay preview after edit"} {
		t.Run(trigger, func(t *testing.T) {
			browser, err := chromium.New(chromiumtest.Start(t))
			require.NoError(t, err)
			tab, closeTab, err := browser.Tab(context.Background())
			require.NoError(t, err)
			ctx, cancel := context.WithTimeout(tab, 30*time.Second)
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

			ro := newTestRouter(t)
			enablePlatformMode(ro)
			ro.cfg.Agent = &agent.Manager{}
			ro.cfg.Relay.Enabled = true
			const session = "preview-regression"
			handler, events := agentBrowserStream(ro, session)
			var started, prompts atomic.Int64
			prompted := make(chan struct{}, 2)
			app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				handler.ServeHTTP(w, r)
				if r.Method == http.MethodPost {
					switch r.URL.Path {
					case "/api/agent/sessions":
						started.Add(1)
					case "/api/agent/sessions/" + session + "/prompt":
						prompts.Add(1)
						prompted <- struct{}{}
					}
				}
			}))
			render := httptest.NewUnstartedServer(nil)
			defer func() { closeTab(); app.CloseClientConnections(); app.Close(); render.Close() }()
			ro.cfg.AppOrigin = app.URL
			ro.cfg.RenderOrigin = "http://" + render.Listener.Addr().String()
			render.Config.Handler = ro.RenderHandler()
			render.Start()
			artifactBody := func(text string) string {
				return fmt.Sprintf(`<!doctype html><h1>%s</h1><script>parent.postMessage({previewMarker: %q}, %q)</script>`, text, text, app.URL)
			}
			var id string
			saveEvent := func(action string) {
				events <- []byte(fmt.Sprintf(`{"type":"exhibit_artifact_saved","artifactId":%q,"title":"Counter","action":%q}`, id, action))
			}
			waitForPrompt := func() {
				t.Helper()
				select {
				case <-prompted:
				case <-ctx.Done():
					t.Fatal("agent prompt did not arrive")
				}
			}
			if trigger == "reload after creation" {
				run(chromedp.Navigate(app.URL+"/new"),
					chromedp.SendKeys(chromedp.CSS("#agent-description"), "Build a click counter"),
				)
				_, err := chromedp.RunResponse(ctx, chromedp.Click(chromedp.CSS("#agent-panel [type=submit]")))
				require.NoError(t, err)
				check(`location.pathname === '/agent' && !!document.getElementById('empty-preview')`)
				waitForPrompt()
				id = createArtifact(t, ro, map[string]any{"title": "Counter", "body": artifactBody("First version")})
				keepConversation(t, ro, id, session, "Build a click counter", `{"type":"session","version":3,"id":"preview-regression","cwd":"/"}
{"type":"message","id":"1","message":{"role":"user","content":"Build a click counter"}}
{"type":"message","id":"2","message":{"role":"assistant","content":[{"type":"text","text":"The counter is ready."}]}}
`)
				saveEvent("created")
				run(chromedp.Poll[chromedp.Void](`!!document.getElementById('pv-frame')?.getAttribute('src')`))
				_, err = chromedp.RunResponse(ctx, chromedp.Reload())
				require.NoError(t, err)
				run(chromedp.Poll[chromedp.Void](`typeof window.send === 'function'`))
				check(fmt.Sprintf(`new URLSearchParams(location.search).get('artifact') === %q &&
					!!document.getElementById('pv-frame')?.getAttribute('src') &&
					document.querySelector('#history-messages')?.textContent.includes('Build a click counter') &&
					document.querySelector('#history-messages')?.textContent.includes('The counter is ready.')`, id))
				require.Equal(t, int64(1), started.Load())
				require.Equal(t, int64(1), prompts.Load())
				return
			}

			id = createArtifact(t, ro, map[string]any{"title": "Counter", "body": artifactBody("First version")})
			run(chromedp.Navigate(app.URL+"/agent?artifact="+id),
				chromedp.Poll[chromedp.Void](`typeof window.send === 'function' && !!document.getElementById('pv-frame')?.getAttribute('src')`),
				chromedp.Evaluate[chromedp.Void](`
					window.oldFrame = document.getElementById('pv-frame');
					window.oldSource = oldFrame.src;
					window.oldPolicy = document.querySelector('meta[data-exhibit-frame-policy]').content;
					window.editSwapSettled = false;
					document.getElementById('pane-preview').addEventListener('htmx:after:swap', () => { window.editSwapSettled = true; });
					window.previewMarker = '';
					window.addEventListener('message', event => {
						if (event.source === document.getElementById('pv-frame')?.contentWindow && event.data?.previewMarker) {
							window.previewMarker = event.data.previewMarker;
						}
					});
				`),
				chromedp.SendKeys(chromedp.CSS("#input"), "Show the second version"),
				chromedp.Click(chromedp.CSS("#send-btn")))
			waitForPrompt()
			saved := doJSON(t, ro, http.MethodPatch, "/api/artifacts/"+id, map[string]any{"body": artifactBody("Second version")})
			require.Equal(t, http.StatusOK, saved.Code, saved.Body.String())
			saveEvent("updated")
			run(chromedp.Poll[chromedp.Void](`window.editSwapSettled === true`))
			check(`document.getElementById('pv-frame') !== oldFrame &&
				!!document.getElementById('pv-frame').getAttribute('src') &&
				document.getElementById('pv-frame').src !== oldSource`)
			run(chromedp.Poll[chromedp.Void](`window.previewMarker === 'Second version'`))
			check(`document.querySelector('meta[data-exhibit-frame-policy]').content === oldPolicy &&
				document.getElementById('messages').textContent.includes('Show the second version') &&
				document.getElementById('messages').textContent.includes('Artifact updated.')`)
		})
	}
}
