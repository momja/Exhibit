package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/momja/Exhibit/internal/chromium"
	"github.com/momja/Exhibit/internal/chromium/chromiumtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelayBrowserResumesThrottledReadAndStopsOnAbortOrNavigation(t *testing.T) {
	browser, err := chromium.New(chromiumtest.Start(t))
	require.NoError(t, err)
	ctx, cancel, err := browser.Tab(context.Background())
	require.NoError(t, err)
	defer cancel()
	ctx, timeout := context.WithTimeout(ctx, 30*time.Second)
	defer timeout()
	runtime, err := os.ReadFile("../render/relay.js")
	require.NoError(t, err)
	var appURL string
	var readCalls, abortCalls, navigationCalls atomic.Int64
	frame := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		if r.URL.Query().Has("replacement") {
			fmt.Fprint(w, "<!doctype html><p>Replacement document</p>")
			return
		}
		mode, _ := json.Marshal(r.URL.Query().Get("mode"))
		app, _ := json.Marshal(appURL)
		script := strings.Replace(string(runtime), "__EXHIBIT_RELAY_CONFIG__", fmt.Sprintf(`{"app_origin":%s,"artifact_id":"app-1","origins":["https://api.example.test"]}`, app), 1)
		fmt.Fprintf(w, `<!doctype html><head><script>%s</script></head><body><script>
const app=%s, mode=%s, controller=new AbortController();
addEventListener('message',event=>{if(event.source===parent&&event.origin===app&&event.data==='abort')controller.abort();});
fetch('https://api.example.test/'+mode,{signal:controller.signal}).then(response=>response.text())
 .then(body=>parent.postMessage({body},app)).catch(error=>parent.postMessage({error:error.name},app));
</script></body>`, script, app, mode)
	}))
	defer frame.Close()
	mux := http.NewServeMux()
	mux.Handle("/assets/", assetsHandler())
	mux.HandleFunc("/api/artifacts/app-1/relay", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "current-policy", r.Header.Get("X-Exhibit-Relay-Policy"))
		var input relayRequest
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		counter := &readCalls
		if strings.HasSuffix(input.URL, "/abort") {
			counter = &abortCalls
		} else if strings.HasSuffix(input.URL, "/navigation") {
			counter = &navigationCalls
		}
		if counter.Add(1) == 1 {
			w.Header().Set("Retry-After", "1")
			writeJSON(w, 429, map[string]string{"code": "relay_rate_limited", "error": "Wait before retrying."})
			return
		}
		writeJSON(w, 200, relayResponse{Status: 200, StatusText: "OK", Headers: map[string]string{}, BodyBase64: base64.StdEncoding.EncodeToString([]byte("resumed")), URL: input.URL})
	})
	mux.HandleFunc("/{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<!doctype html><body>
<div id="host" data-controller="relay-host" data-relay-host-artifact-id-value="app-1" data-relay-host-frame-selector-value="#frame" data-relay-host-policy-value="current-policy">
<iframe id="frame" sandbox="allow-scripts" data-relay-src="%s/a/app-1?mode=%s"></iframe></div>
<script>const TOKEN='',READ_ONLY=false;addEventListener('message',event=>{window.result=event.data});</script>
<script src="/assets/gallery/api.js"></script><script type="module">import {application} from '/assets/gallery/controllers.js';window.application=application;</script>
</body>`, frame.URL, r.URL.Query().Get("mode"))
	})
	app := httptest.NewServer(mux)
	defer app.Close()
	appURL = app.URL
	run := func(actions ...chromedp.Action[chromedp.Void]) {
		t.Helper()
		require.NoError(t, chromedp.Do(ctx, actions...))
	}
	run(chromedp.Navigate(appURL+"?mode=read"), chromedp.Poll[chromedp.Void](`window.result?.body === 'resumed'`))
	assert.EqualValues(t, 2, readCalls.Load(), "the frame receives a successful response after the local throttle")

	run(chromedp.Navigate(appURL + "?mode=abort"))
	require.Eventually(t, func() bool { return abortCalls.Load() == 1 }, 5*time.Second, 10*time.Millisecond)
	run(chromedp.Evaluate[chromedp.Void](`document.getElementById('frame').contentWindow.postMessage('abort','*')`),
		chromedp.Poll[chromedp.Void](`window.result?.error === 'AbortError'`), chromedp.Sleep(1200*time.Millisecond))
	assert.EqualValues(t, 1, abortCalls.Load(), "aborting the guest fetch stops its delayed host retry")

	run(chromedp.Navigate(appURL + "?mode=navigation"))
	require.Eventually(t, func() bool { return navigationCalls.Load() == 1 }, 5*time.Second, 10*time.Millisecond)
	replacement, _ := json.Marshal(frame.URL + "/a/app-1?replacement=1")
	run(chromedp.Evaluate[chromedp.Void](`document.getElementById('frame').src=`+string(replacement)),
		chromedp.Poll[chromedp.Void](`application.getControllerForElementAndIdentifier(document.getElementById('host'),'relay-host').requests.size === 0`), chromedp.Sleep(1200*time.Millisecond))
	assert.EqualValues(t, 1, navigationCalls.Load(), "a new document cannot inherit the old document's queued retry")
}
