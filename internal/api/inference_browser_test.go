package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/momja/Exhibit/internal/chromium"
	"github.com/momja/Exhibit/internal/chromium/chromiumtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The real bridge and the real host, in a browser (Exh-zbwk). The artifact
// asks available() at the top of its script, before the host page's module
// graph has run — forced here by holding back inference-host.js — and the call
// must still settle with the true answer rather than be lost. Then the owner's
// first complete() raises the dialog in Exhibit's chrome, Allow records the
// approval, and the answer reaches the artifact, which saves both results so
// the test can read them through the state API.
func TestInferenceBridgeBrowser(t *testing.T) {
	browser, err := chromium.New(chromiumtest.Start(t))
	require.NoError(t, err)
	ctx, cancel, err := browser.Tab(context.Background())
	require.NoError(t, err)
	defer cancel()
	ctx, timeout := context.WithTimeout(ctx, 60*time.Second)
	defer timeout()

	ro := newTestRouter(t)
	rt := withPlatformInference(t, ro)
	body := `<!doctype html><html><head><script>
window.exhibit.ai.available().then(function (a) {
  localStorage.setItem('available', JSON.stringify(a));
  return window.exhibit.ai.complete({ prompt: 'Summarize' });
}).then(function (text) {
  localStorage.setItem('answer', text);
}, function (e) {
  localStorage.setItem('answer', 'error:' + e.code);
});
</script></head><body><h1>AI tool</h1></body></html>`
	id := createArtifact(t, ro, map[string]any{"title": "AI tool", "body": body})

	appMux, renderMux := http.NewServeMux(), http.NewServeMux()
	app := httptest.NewServer(appMux)
	defer app.Close()
	frame := httptest.NewServer(renderMux)
	defer frame.Close()
	ro.cfg.AppOrigin, ro.cfg.RenderOrigin = app.URL, frame.URL
	renderMux.Handle("/", ro.RenderHandler())
	appMux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/inference-host.js") {
			time.Sleep(1500 * time.Millisecond) // the frame calls first
		}
		r.Header.Set("Authorization", authHeader())
		ro.ServeHTTP(w, r)
	}))

	require.NoError(t, chromedp.Do(ctx,
		chromedp.Navigate(app.URL+"/artifacts/"+id),
		chromedp.Poll[chromedp.Void](`document.getElementById('ai-modal')?.open && document.getElementById('ai-modal').dataset.aiMode === 'owner'`),
	))
	assert.Equal(t, `{"complete":true,"decide":true}`, getState(t, ro, id)["available"],
		"a call made before the host listened still settles, with the real answer")
	assert.Zero(t, rt.calls(), "nothing runs before the owner allows it")

	require.NoError(t, chromedp.Do(ctx, chromedp.Click(chromedp.CSS("#ai-allow"))))
	require.Eventually(t, func() bool { return getState(t, ro, id)["answer"] != "" }, 10*time.Second, 50*time.Millisecond)
	assert.Equal(t, "a summary", getState(t, ro, id)["answer"])
	assert.Equal(t, 1, rt.calls(), "one call, sent once")
	a, err := ro.cfg.Store.GetArtifact(t.Context(), 1, id)
	require.NoError(t, err)
	assert.True(t, a.InferenceApproved, "Allow is the owner's approval")
}

// "Not now" holds for the page load: a tool that asks again straight away is
// refused without the dialog coming back, so a loop cannot trap the person.
func TestInferenceNotNowIsRememberedForTheLoadBrowser(t *testing.T) {
	browser, err := chromium.New(chromiumtest.Start(t))
	require.NoError(t, err)
	ctx, cancel, err := browser.Tab(context.Background())
	require.NoError(t, err)
	defer cancel()
	ctx, timeout := context.WithTimeout(ctx, 60*time.Second)
	defer timeout()

	ro := newTestRouter(t)
	rt := withPlatformInference(t, ro)
	body := `<!doctype html><html><head><script>
function ask() { return window.exhibit.ai.complete({ prompt: 'Summarize' }).then(function () { return 'ok'; }, function (e) { return e.code; }); }
ask().then(function (first) {
  localStorage.setItem('first', first);
  return ask();
}).then(function (second) { localStorage.setItem('second', second); });
</script></head><body><h1>Loop</h1></body></html>`
	id := createArtifact(t, ro, map[string]any{"title": "Loop", "body": body})
	appMux, renderMux := http.NewServeMux(), http.NewServeMux()
	app := httptest.NewServer(appMux)
	defer app.Close()
	frame := httptest.NewServer(renderMux)
	defer frame.Close()
	ro.cfg.AppOrigin, ro.cfg.RenderOrigin = app.URL, frame.URL
	renderMux.Handle("/", ro.RenderHandler())
	appMux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Header.Set("Authorization", authHeader())
		ro.ServeHTTP(w, r)
	}))

	require.NoError(t, chromedp.Do(ctx,
		chromedp.Navigate(app.URL+"/artifacts/"+id),
		chromedp.Poll[chromedp.Void](`document.getElementById('ai-modal')?.open`),
		chromedp.Click(chromedp.CSS("#ai-block")),
	))
	require.Eventually(t, func() bool { return getState(t, ro, id)["second"] != "" }, 10*time.Second, 50*time.Millisecond)
	assert.Equal(t, "not_approved", getState(t, ro, id)["first"])
	assert.Equal(t, "not_approved", getState(t, ro, id)["second"])
	open, err := chromedp.Run(ctx, chromedp.Evaluate[bool](`document.getElementById('ai-modal').open`))
	require.NoError(t, err)
	assert.False(t, open, "the second call does not raise the dialog again")
	assert.Zero(t, rt.calls())
}
