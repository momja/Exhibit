package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync"
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
	ro := newTestRouter(t)
	ro.cfg.Inference, err = inference.New(inference.Config{
		Enabled: true, Platform: &inference.Credential{Provider: "openrouter", APIKey: "test-platform-key"},
		ChatModel: model, DecisionModel: "typesafe/jev-1.14", Ledger: ro.cfg.Store, Runtime: worker,
		Guardrail: &inference.Guardrail{Provider: "openrouter", Model: model, APIKey: "test-guard-key"},
	})
	require.NoError(t, err)
	id := createArtifact(t, ro, map[string]any{"title": "Feedback triage", "body": triageExampleSource(t)})
	app := serveInferenceBrowser(t, ro, 0)

	browser, err := chromium.New(chromiumtest.Start(t))
	require.NoError(t, err)
	ctx, cancel, err := browser.Tab(t.Context())
	require.NoError(t, err)
	defer cancel()
	ctx, timeout := context.WithTimeout(ctx, 60*time.Second)
	defer timeout()
	require.NoError(t, chromedp.Do(ctx, chromedp.Navigate(app.URL+"/artifacts/"+id)))
	frames, err := chromedp.Run(ctx, chromedp.Nodes(chromedp.CSS("iframe[data-ai-frame]"), chromedp.AtLeast(1)))
	require.NoError(t, err)
	frame := chromedp.FromNode(frames[0])
	inFrame := chromedp.WithPollingInFrame(frames[0])
	require.NoError(t, chromedp.Do(ctx,
		chromedp.SetValue(chromedp.CSS("#new-items"), "The app crashes on sign-in.", frame),
		chromedp.Click(chromedp.CSS("#add-items"), frame),
		chromedp.Poll[chromedp.Void](`(() => {
			const button = document.getElementById('triage-ai');
			if (!button || button.disabled) return false;
			button.click();
			document.getElementById('preview-send').click();
			return true;
		})()`, inFrame),
	))
	require.NoError(t, chromedp.Do(ctx, chromedp.Poll[chromedp.Void](`document.getElementById('ai-modal').open`)), "owner prompt opens")
	assert.Zero(t, calls.Load(), "the example cannot reach the mock before approval")
	require.NoError(t, chromedp.Do(ctx,
		chromedp.Click(chromedp.CSS("#ai-allow")),
		chromedp.Poll[chromedp.Void](`(() => {
			if (!document.querySelector('[data-action="save-triage"]:not([hidden])')) return false;
			const button = document.getElementById('summary-ai');
			if (button.disabled) return false;
			button.click();
			document.getElementById('preview-send').click();
			return true;
		})()`, inFrame),
		chromedp.Poll[chromedp.Void](`document.getElementById('summary-meta').textContent.startsWith('Private draft')`, inFrame),
	))
	assert.Equal(t, int32(4), calls.Load(), "two screens, one classifier dispatch and one completion")
}

func triageExampleSource(t *testing.T) string {
	t.Helper()
	source, err := os.ReadFile("../../examples/inference-triage/index.html")
	require.NoError(t, err)
	return string(source)
}

func serveInferenceBrowser(t *testing.T, ro *Router, hostDelay time.Duration) *httptest.Server {
	t.Helper()
	appMux, renderMux := http.NewServeMux(), http.NewServeMux()
	app, frame := httptest.NewServer(appMux), httptest.NewServer(renderMux)
	t.Cleanup(app.Close)
	t.Cleanup(frame.Close)
	ro.cfg.AppOrigin, ro.cfg.RenderOrigin = app.URL, frame.URL
	renderMux.Handle("/", ro.RenderHandler())
	appMux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hostDelay > 0 && strings.HasSuffix(r.URL.Path, "/inference-host.js") {
			time.Sleep(hostDelay)
		}
		r.Header.Set("Authorization", authHeader())
		ro.ServeHTTP(w, r)
	}))
	return app
}

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

	app := serveInferenceBrowser(t, ro, 1500*time.Millisecond) // the frame calls first

	require.NoError(t, chromedp.Do(ctx,
		chromedp.Navigate(app.URL+"/artifacts/"+id),
		chromedp.Poll[chromedp.Void](`document.getElementById('ai-modal')?.open && document.getElementById('ai-modal').dataset.aiMode === 'owner'`),
	))
	assert.Equal(t, `{"complete":true,"decide":true,"images":true}`, getState(t, ro, id)["available"],
		"a call made before the host listened still settles, with the real answer")
	assert.Zero(t, rt.calls(), "nothing runs before the owner allows it")

	require.NoError(t, chromedp.Do(ctx, chromedp.Click(chromedp.CSS("#ai-allow"))))
	require.Eventually(t, func() bool { return getState(t, ro, id)["answer"] != "" }, 10*time.Second, 50*time.Millisecond)
	assert.Equal(t, "a summary", getState(t, ro, id)["answer"])
	assert.Equal(t, "false", getState(t, ro, id)["windowReceivedAIResult"],
		"AI responses stay on the requesting document's ports, off the navigable frame window")
	assert.Equal(t, 1, rt.calls(), "one call, sent once")
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
	app := serveInferenceBrowser(t, ro, 0)

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

// An image from a canvas crosses the real bridge (Exh-v81w): the frame's
// structured clone, the host's request and the endpoint, at a size well past
// what complete() took before images, and arrives as an image part.
func TestInferenceBridgeCarriesACanvasImageBrowser(t *testing.T) {
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
var canvas = document.createElement('canvas');
canvas.width = 900; canvas.height = 900;
var g = canvas.getContext('2d'), px = g.createImageData(900, 900);
for (var i = 0; i < px.data.length; i++) px.data[i] = (i * 2654435761) % 256; // noise compresses badly
g.putImageData(px, 0, 0);
var dataURL = canvas.toDataURL('image/jpeg', 0.9);
window.exhibit.ai.available().then(function (a) {
  if (!a.images) throw Object.assign(new Error('no images'), { code: 'no_images' });
  return window.exhibit.ai.complete({ messages: [{ role: 'user', content: [
    { type: 'text', text: 'Describe this.' },
    { type: 'image', mimeType: 'image/jpeg', data: dataURL },
  ] }] });
}).then(function (text) { localStorage.setItem('answer', text); },
  function (e) { localStorage.setItem('answer', 'error:' + e.code); });
</script></head><body><h1>Image tool</h1></body></html>`
	id := createArtifact(t, ro, map[string]any{"title": "Image tool", "body": body})
	require.NoError(t, ro.cfg.Store.UpdateArtifact(t.Context(), 1, id, map[string]any{"inference_approved": true}))

	app := serveInferenceBrowser(t, ro, 0)
	require.NoError(t, chromedp.Do(ctx, chromedp.Navigate(app.URL+"/artifacts/"+id)))
	require.Eventually(t, func() bool { return getState(t, ro, id)["answer"] != "" }, 20*time.Second, 50*time.Millisecond)
	assert.Equal(t, "a summary", getState(t, ro, id)["answer"])
	require.Equal(t, 1, rt.calls())
	job := string(rt.jobs[0].Context)
	assert.Contains(t, job, `"mimeType":"image/jpeg"`)
	assert.Greater(t, len(job), 256<<10, "the canvas image should exceed the old 256 KB envelope")
}

// The host page strips an image's metadata before the request leaves the
// browser (Exh-v81w): a canvas JPEG with an EXIF segment spliced in arrives at
// the endpoint, and at the runtime, without it. Every call the server would
// refuse for its images or their keys (a JPEG cut short, a GIF, a mislabelled
// file, a key in another case, too many bytes in total as given) is settled
// by the host as invalid_request in the server's words, and never sent.
func TestInferenceHostStripsImageMetadataBrowser(t *testing.T) {
	browser, err := chromium.New(chromiumtest.Start(t))
	require.NoError(t, err)
	ctx, cancel, err := browser.Tab(context.Background())
	require.NoError(t, err)
	defer cancel()
	ctx, timeout := context.WithTimeout(ctx, 60*time.Second)
	defer timeout()

	const marker = "GPS-MARKER-v81w"
	ro := newTestRouter(t)
	rt := withPlatformInference(t, ro)
	body := `<!doctype html><html><head><script>
var canvas = document.createElement('canvas');
canvas.width = 64; canvas.height = 64;
canvas.getContext('2d').fillRect(8, 8, 32, 32);
var jpeg = atob(canvas.toDataURL('image/jpeg', 0.9).split(',')[1]);
var exif = 'Exif\0\0MM\0*GPSLatitude ` + marker + `';
var app1 = '\xff\xe1' + String.fromCharCode((exif.length + 2) >> 8, (exif.length + 2) & 255) + exif;
var tagged = btoa(jpeg.slice(0, 2) + app1 + jpeg.slice(2));
function send(data) {
  return window.exhibit.ai.complete({ messages: [{ role: 'user', content: [
    { type: 'text', text: 'Describe this.' },
    { type: 'image', mimeType: 'image/jpeg', data: data },
  ] }] }).then(function (text) { return text; }, function (e) { return e.code + ': ' + e.message; });
}
function settle(payload) {
  return window.exhibit.ai.complete(payload).then(function () { return 'sent'; }, function (e) { return e.code + ': ' + e.message; });
}
// A well-formed PNG of n bytes: IHDR, one IDAT of zeros, IEND.
function png(n) {
  var u32 = function (v) { return String.fromCharCode(v >>> 24, (v >>> 16) & 255, (v >>> 8) & 255, v & 255); };
  return btoa('\x89PNG\r\n\x1a\n' + u32(13) + 'IHDR' + '\0\0\0\x01\0\0\0\x01\x08\0\0\0\0' + u32(0) +
    u32(n - 57) + 'IDAT' + '\0'.repeat(n - 57) + u32(0) + u32(0) + 'IEND' + u32(0));
}
var image = function (mimeType, data) { return { type: 'image', mimeType: mimeType, data: data }; };
var user = function (parts) { return { messages: [{ role: 'user', content: parts }] }; };
send(tagged).then(function (answer) {
  localStorage.setItem('answer', answer);
  return send(btoa(jpeg.slice(0, 2) + app1 + jpeg.slice(2, 40)));
}).then(function (broken) {
  localStorage.setItem('broken', broken);
  return Promise.all([
    settle(user([image('image/gif', 'R0lGODlhAQABAAAAACw=')])),
    settle(user([image('image/png', tagged)])),
    settle({ Messages: [{ role: 'user', content: [image('image/jpeg', tagged)] }] }),
    settle(user([{ type: 'image', mimeType: 'image/jpeg', Data: tagged }])),
    settle(user([image('image/png', png(1500000)), image('image/png', png(1500000)), image('image/png', png(1500000))])),
  ]);
}).then(function (refused) { localStorage.setItem('refused', JSON.stringify(refused)); });
</script></head><body><h1>Photo tool</h1></body></html>`
	id := createArtifact(t, ro, map[string]any{"title": "Photo tool", "body": body})
	require.NoError(t, ro.cfg.Store.UpdateArtifact(t.Context(), 1, id, map[string]any{"inference_approved": true}))

	// What the host page actually sends, before the server's own strip.
	var mu sync.Mutex
	var sent []string
	appMux, renderMux := http.NewServeMux(), http.NewServeMux()
	app, frame := httptest.NewServer(appMux), httptest.NewServer(renderMux)
	t.Cleanup(app.Close)
	t.Cleanup(frame.Close)
	ro.cfg.AppOrigin, ro.cfg.RenderOrigin = app.URL, frame.URL
	renderMux.Handle("/", ro.RenderHandler())
	appMux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/inference") {
			raw, _ := io.ReadAll(r.Body)
			mu.Lock()
			sent = append(sent, string(raw))
			mu.Unlock()
			r.Body = io.NopCloser(bytes.NewReader(raw))
		}
		r.Header.Set("Authorization", authHeader())
		ro.ServeHTTP(w, r)
	}))

	require.NoError(t, chromedp.Do(ctx, chromedp.Navigate(app.URL+"/artifacts/"+id)))
	require.Eventually(t, func() bool { return getState(t, ro, id)["refused"] != "" }, 20*time.Second, 50*time.Millisecond)
	assert.Equal(t, "a summary", getState(t, ro, id)["answer"])
	assert.Equal(t, "invalid_request: An image's data is not a well-formed image/jpeg file.", getState(t, ro, id)["broken"])
	var refused []string
	require.NoError(t, json.Unmarshal([]byte(getState(t, ro, id)["refused"]), &refused))
	assert.Equal(t, []string{
		"invalid_request: Images must be PNG, JPEG or WebP (mimeType image/png, image/jpeg or image/webp).",
		"invalid_request: An image's data is not the image/png its mimeType says.",
		"invalid_request: complete() takes { messages, system?, schema?, maxTokens? }.",
		"invalid_request: A message's content must be text, or a list of { type: 'text', text } and { type: 'image', mimeType, data } parts.",
		"invalid_request: complete() images are limited to 4 MB in total. Downscale them in a canvas before sending.",
	}, refused)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, sent, 1, "only the well-formed call left the browser")
	// The image bytes in a JSON document, wherever the parts list sits.
	imageBytes := func(doc string) []byte {
		var parts []struct{ Type, Data string }
		var sentBody struct {
			Payload struct {
				Messages []struct{ Content json.RawMessage } `json:"messages"`
			} `json:"payload"`
			Messages []struct{ Content json.RawMessage } `json:"messages"`
		}
		require.NoError(t, json.Unmarshal([]byte(doc), &sentBody))
		msgs := append(sentBody.Payload.Messages, sentBody.Messages...)
		require.Len(t, msgs, 1)
		require.NoError(t, json.Unmarshal(msgs[0].Content, &parts))
		require.Len(t, parts, 2)
		require.Equal(t, "image", parts[1].Type)
		raw, err := base64.StdEncoding.DecodeString(parts[1].Data)
		require.NoError(t, err, "the host sends plain base64")
		return raw
	}
	fromHost := imageBytes(sent[0])
	assert.True(t, bytes.HasPrefix(fromHost, []byte{0xFF, 0xD8, 0xFF}))
	assert.NotContains(t, string(fromHost), marker, "the host sent the EXIF segment")
	assert.NotContains(t, string(fromHost), "Exif\x00\x00")
	require.Equal(t, 1, rt.calls())
	assert.Equal(t, fromHost, imageBytes(string(rt.jobs[0].Context)), "the server's strip finds nothing left to remove")
	assert.Contains(t, string(rt.jobs[0].Context), `"mimeType":"image/jpeg"`)
}

// A request over the instance's body limit is refused by the body-limit
// middleware before the inference handler, in that middleware's own shape.
// The artifact gets invalid_request with a sentence it can act on, not a
// provider_error (Exh-v81w review).
func TestInferenceBridgeReportsAnOversizedRequestBrowser(t *testing.T) {
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
var canvas = document.createElement('canvas');
canvas.width = 900; canvas.height = 900;
var g = canvas.getContext('2d'), px = g.createImageData(900, 900);
for (var i = 0; i < px.data.length; i++) px.data[i] = (i * 2654435761) % 256;
g.putImageData(px, 0, 0);
window.exhibit.ai.complete({ messages: [{ role: 'user', content: [
  { type: 'text', text: 'Describe this.' },
  { type: 'image', mimeType: 'image/jpeg', data: canvas.toDataURL('image/jpeg', 0.9) },
] }] }).then(function () { localStorage.setItem('answer', 'ok'); },
  function (e) { localStorage.setItem('answer', e.code + ': ' + e.message); });
</script></head><body><h1>Image tool</h1></body></html>`
	id := createArtifact(t, ro, map[string]any{"title": "Image tool", "body": body})
	require.NoError(t, ro.cfg.Store.UpdateArtifact(t.Context(), 1, id, map[string]any{"inference_approved": true}))

	// The real middleware, with a limit the image overruns.
	limited := limitRequestBody(256 << 10)(ro)
	appMux, renderMux := http.NewServeMux(), http.NewServeMux()
	app, frame := httptest.NewServer(appMux), httptest.NewServer(renderMux)
	t.Cleanup(app.Close)
	t.Cleanup(frame.Close)
	ro.cfg.AppOrigin, ro.cfg.RenderOrigin = app.URL, frame.URL
	renderMux.Handle("/", ro.RenderHandler())
	appMux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Header.Set("Authorization", authHeader())
		limited.ServeHTTP(w, r)
	}))

	require.NoError(t, chromedp.Do(ctx, chromedp.Navigate(app.URL+"/artifacts/"+id)))
	require.Eventually(t, func() bool { return getState(t, ro, id)["answer"] != "" }, 20*time.Second, 50*time.Millisecond)
	assert.Equal(t, "invalid_request: This AI request is too large for this Exhibit. Send less with it, such as fewer or smaller images.", getState(t, ro, id)["answer"])
	assert.Zero(t, rt.calls())
}
