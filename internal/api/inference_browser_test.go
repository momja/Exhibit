package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/momja/Exhibit/internal/chromium"
	"github.com/momja/Exhibit/internal/chromium/chromiumtest"
	"github.com/momja/Exhibit/internal/inference"
	"github.com/momja/Exhibit/internal/mockllm"
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
	exerciseInferenceBridgeBrowser(t, func(t *testing.T, ro *Router) func() int {
		return withPlatformInference(t, ro).calls
	}, "a summary", 1)
}

func TestInferenceBridgeThroughMockProviderBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("Chromium integration disabled in short test runs")
	}
	piBin := os.Getenv("PI_BIN")
	if piBin == "" {
		piBin = "pi"
	}
	piBin, err := exec.LookPath(piBin)
	if err != nil {
		t.Skip("no Pi executable; set PI_BIN to the pinned Pi Bolt binary")
	}
	exerciseInferenceBridgeBrowser(t, func(t *testing.T, ro *Router) func() int {
		var calls atomic.Int32
		handler := mockllm.Handler()
		provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			handler.ServeHTTP(w, r)
		}))
		t.Cleanup(provider.Close)
		worker, err := inference.NewWorker(inference.WorkerConfig{PiBin: piBin, WorkDir: t.TempDir(), MockURL: provider.URL})
		require.NoError(t, err)
		t.Cleanup(worker.Close)
		const model = "openai/gpt-4.1-nano"
		ro.cfg.Inference, err = inference.New(inference.Config{
			Enabled: true, Platform: &inference.Credential{Provider: "openrouter", APIKey: "test-platform-key"},
			ChatModel: model, DecisionModel: "typesafe/jev-1.14", Ledger: ro.cfg.Store, Runtime: worker,
			Guardrail: &inference.Guardrail{Provider: "openrouter", Model: model, APIKey: "test-guard-key"},
		})
		require.NoError(t, err)
		return func() int { return int(calls.Load()) }
	}, "Summary: Summarize", 2)
}

func exerciseInferenceBridgeBrowser(t *testing.T, configure func(*testing.T, *Router) func() int, wantAnswer string, wantCalls int) {
	t.Helper()
	browser, err := chromium.New(chromiumtest.Start(t))
	require.NoError(t, err)
	ctx, cancel, err := browser.Tab(context.Background())
	require.NoError(t, err)
	defer cancel()
	ctx, timeout := context.WithTimeout(ctx, 60*time.Second)
	defer timeout()

	ro := newTestRouter(t)
	calls := configure(t, ro)
	body := `<!doctype html><html><head><script>
var windowReceivedAIResult = false;
window.addEventListener('message', function (event) {
  if (event.data?.__avInferenceResult) windowReceivedAIResult = true;
});
window.addEventListener('storage', function (event) {
  if (event.key === 'resync') localStorage.setItem('syncedValue', event.newValue);
});
window.exhibit.ai.available().then(function (a) {
  localStorage.setItem('available', JSON.stringify(a));
  return window.exhibit.ai.complete({ prompt: 'Summarize' });
}).then(function (text) {
  localStorage.setItem('windowReceivedAIResult', String(windowReceivedAIResult));
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
	assert.Zero(t, calls(), "nothing runs before the owner allows it")

	require.NoError(t, chromedp.Do(ctx, chromedp.Click(chromedp.CSS("#ai-allow"))))
	require.Eventually(t, func() bool { return getState(t, ro, id)["answer"] != "" }, 10*time.Second, 50*time.Millisecond)
	assert.Equal(t, wantAnswer, getState(t, ro, id)["answer"])
	assert.Equal(t, "false", getState(t, ro, id)["windowReceivedAIResult"],
		"AI responses stay on the requesting document's ports, off the navigable frame window")
	assert.Equal(t, wantCalls, calls(), "the completion and any configured screen are each sent once")
	a, err := ro.cfg.Store.GetArtifact(t.Context(), 1, id)
	require.NoError(t, err)
	assert.True(t, a.InferenceApproved, "Allow is the owner's approval")

	require.NoError(t, ro.cfg.Store.SetStateAsViewer(ctx, 1, id, "resync", "from another device"))
	_, err = chromedp.Run(ctx, chromedp.Evaluate[bool](`document.dispatchEvent(new Event('visibilitychange'))`))
	require.NoError(t, err)
	require.Eventually(t, func() bool { return getState(t, ro, id)["syncedValue"] == "from another device" }, 10*time.Second, 50*time.Millisecond)
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
