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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/momja/Exhibit/internal/chromium"
	"github.com/momja/Exhibit/internal/chromium/chromiumtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This isolates the real host controller and render preamble in separate
// browser origins. The API returns fixture bytes; relay_test.go exercises the
// server's real egress/consent checks. Here the adversary is frame navigation,
// forged window messages and HTML in an API response.
func TestRelayBrowserBindsChannelsToDocumentsAndNeverRendersAPIBytes(t *testing.T) {
	browser, err := chromium.New(chromiumtest.Start(t))
	require.NoError(t, err)
	ctx, cancel, err := browser.Tab(context.Background())
	require.NoError(t, err)
	defer cancel()
	ctx, timeout := context.WithTimeout(ctx, 30*time.Second)
	defer timeout()
	runtime, err := os.ReadFile("../render/relay.js")
	require.NoError(t, err)
	const payload = `<script>window.appOriginCompromised=true</script><img id="upstream-unsafe" src=x>`
	var calls, evilCalls atomic.Int64
	var heldStarted atomic.Bool
	var badPolicy atomic.Bool
	var appURL string
	held := make(chan struct{})
	releaseHeld := sync.OnceFunc(func() { close(held) })
	frameServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if r.URL.Path != "/a/app-1" {
			evilCalls.Add(1)
			fmt.Fprint(w, "another artifact with a broader network policy")
			return
		}
		if r.URL.Query().Has("held") {
			heldStarted.Store(true)
			select {
			case <-held:
			case <-r.Context().Done():
				return
			}
		}
		appJSON, _ := json.Marshal(appURL)
		if r.URL.Query().Has("replacement") || r.URL.Query().Has("imposter") {
			fmt.Fprintf(w, `<!doctype html><script>
const app=%s;const channel=new MessageChannel();
channel.port1.onmessage=event=>parent.postMessage({testLeaked:true,value:event.data},app);
parent.postMessage({__exhibitRelayChannel:true},app,[channel.port2]);
channel.port1.postMessage({__exhibitRelay:true,id:'replacement-forgery',request:{url:'https://api.example.test/forged',method:'GET',headers:{},body_base64:''}});
parent.postMessage({testReplacement:true},app);
</script>`, appJSON)
			return
		}
		config, _ := json.Marshal(map[string]any{"app_origin": appURL, "origins": []string{"https://api.example.test"}, "artifact_id": "app-1"})
		script := strings.Replace(string(runtime), "__EXHIBIT_RELAY_CONFIG__", string(config), 1)
		fmt.Fprintf(w, `<!doctype html><head><script>%s</script></head><body><script>
const app=%s;
Promise.all([
 fetch('https://api.example.test/data').then(response=>response.text()),
 window.exhibit.connections.list(),
 window.exhibit.connections.get('connection-1')
]).then(value=>parent.postMessage({testReady:true,body:value[0],connections:value[1],standin:value[2]},app))
 .catch(error=>parent.postMessage({testError:error.message},app));
addEventListener('message',event=>{
 if(event.source!==parent||event.origin!==app)return;
 if(event.data.command==='navigate')location.href=event.data.url;
 if(event.data.command==='forge'){
  parent.postMessage({__exhibitRelay:true,id:'window-forgery',request:{url:'https://api.example.test/forged',method:'GET',body_base64:''}},app);
  const extra=new MessageChannel();parent.postMessage({__exhibitRelayChannel:true},app,[extra.port2]);
  extra.port1.postMessage({__exhibitRelay:true,id:'duplicate-forgery',request:{url:'https://api.example.test/forged',method:'GET',body_base64:''}});
  parent.postMessage({testForged:true},app);
 }
});
</script></body>`, script, appJSON)
	}))
	defer frameServer.Close()
	defer releaseHeld()
	mux := http.NewServeMux()
	mux.Handle("/assets/", assetsHandler())
	mux.HandleFunc("/api/artifacts/app-1/relay", func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("X-Exhibit-Relay-Policy") != "app-policy" {
			badPolicy.Store(true)
		}
		writeJSON(w, 200, relayResponse{Status: 200, StatusText: "OK", Headers: map[string]string{"Content-Type": "text/html"}, BodyBase64: base64.StdEncoding.EncodeToString([]byte(payload)), URL: "https://api.example.test/data"})
	})
	mux.HandleFunc("/api/artifacts/app-1/connections", func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("X-Exhibit-Relay-Policy") != "app-policy" {
			badPolicy.Store(true)
		}
		writeJSON(w, 200, map[string]any{"grants": []map[string]any{{"connection": map[string]any{"id": "connection-1", "name": "Budget", "provider": "simplefin", "origins": []string{"https://api.example.test"}, "endpoint": "/accounts"}}}})
	})
	mux.HandleFunc("/api/artifacts/app-1/connections/connection-1/standin", func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("X-Exhibit-Relay-Policy") != "app-policy" {
			badPolicy.Store(true)
		}
		writeJSON(w, 200, map[string]any{"value": "placeholder-handle", "access_url": "https://api.example.test/accounts", "expires_at": time.Now().Add(time.Minute).Unix()})
	})
	mux.HandleFunc("/{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		// The initially unbound agent page has a broad render-path header. Its
		// first saved artifact supplies an additional exact policy, installed
		// before the deferred frame starts loading.
		w.Header().Set("Content-Security-Policy", relayFramePolicy(frameServer.URL, ""))
		fmt.Fprintf(w, `<!doctype html><body>
<div id="relay-root" data-controller="relay-host" data-relay-host-artifact-id-value="app-1" data-relay-host-frame-selector-value="#artifact-frame" data-relay-host-policy-value="app-policy">
<template data-relay-host-target="framePolicy"><meta http-equiv="Content-Security-Policy" content="%s"></template>
<iframe id="artifact-frame" sandbox="allow-scripts" data-relay-src="%s/a/app-1"></iframe></div>
<script>const TOKEN='';const READ_ONLY=false;window.relayResults=[];window.cspViolations=[];
addEventListener('securitypolicyviolation',event=>cspViolations.push(event.effectiveDirective));
addEventListener('message',event=>{
 if(event.data.testReady)relayResults.push(event.data);
 if(event.data.testError)window.relayError=event.data.testError;
 if(event.data.testReplacement)window.replacementLoaded=true;
 if(event.data.testForged)window.forged=true;
 if(event.data.testLeaked)window.channelLeaked=true;
});</script>
<script src="/assets/gallery/api.js"></script>
<script type="module">import {application} from '/assets/gallery/controllers.js';window.application=application;</script></body>`, relayFramePolicy(frameServer.URL, "app-1"), frameServer.URL)
	})
	appServer := httptest.NewServer(mux)
	defer appServer.Close()
	appURL = appServer.URL
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
	run(chromedp.Navigate(appServer.URL), chromedp.Poll[chromedp.Void](`window.relayResults.length === 1 || window.relayError`))
	check(`!window.relayError && relayResults[0].body.includes('appOriginCompromised') && relayResults[0].connections[0].name === 'Budget' && relayResults[0].standin.value === 'placeholder-handle'`)
	check(`!window.appOriginCompromised && !document.getElementById('upstream-unsafe')`)
	assert.EqualValues(t, 3, calls.Load())
	assert.False(t, badPolicy.Load(), "only the app-side policy is sent with a relay call")

	// Raw window messages and a second channel from the authorized frame never
	// widen the one document-bound channel selected by the trusted preamble.
	run(chromedp.Evaluate[chromedp.Void](`document.getElementById('artifact-frame').contentWindow.postMessage({command:'forge'},'*')`),
		chromedp.Poll[chromedp.Void](`window.forged === true`))
	assert.EqualValues(t, 3, calls.Load())

	// An unrelated opaque frame cannot become the authorized source, even
	// while the real frame's next document is still loading and the controller
	// is waiting for its first channel. Holding that response makes the race
	// deterministic rather than testing only an already-bound controller.
	heldJSON, _ := json.Marshal(frameServer.URL + "/a/app-1?held=1")
	run(chromedp.Evaluate[chromedp.Void](`document.getElementById('artifact-frame').src=` + string(heldJSON)))
	require.Eventually(t, heldStarted.Load, 5*time.Second, 10*time.Millisecond)
	imposterJSON, _ := json.Marshal(frameServer.URL + "/a/app-1?imposter=1")
	run(chromedp.Evaluate[chromedp.Void](`window.replacementLoaded=false;const imposter=document.createElement('iframe');imposter.sandbox='allow-scripts';imposter.src=`+string(imposterJSON)+`;document.body.appendChild(imposter);`),
		chromedp.Poll[chromedp.Void](`window.replacementLoaded === true`))
	assert.EqualValues(t, 3, calls.Load())
	check(`(()=>{const controller=application.getControllerForElementAndIdentifier(document.getElementById('relay-root'),'relay-host');return !controller.port && controller.acceptChannel;})()`)
	releaseHeld()
	run(chromedp.Poll[chromedp.Void](`window.relayResults.length === 2 || window.relayError`))
	check(`!window.relayError && relayResults.length === 2`)
	assert.EqualValues(t, 6, calls.Load())

	// A self-navigation within an allowed path does not inherit the previous
	// document's relay capability or let its replacement open a fresh channel.
	replacementJSON, _ := json.Marshal(frameServer.URL + "/a/app-1?replacement=1")
	run(chromedp.Evaluate[chromedp.Void](`window.replacementLoaded=false;document.getElementById('artifact-frame').contentWindow.postMessage({command:'navigate',url:`+string(replacementJSON)+`},'*')`),
		chromedp.Poll[chromedp.Void](`window.replacementLoaded && !application.getControllerForElementAndIdentifier(document.getElementById('relay-root'),'relay-host').port`))
	assert.EqualValues(t, 6, calls.Load())
	check(`!window.channelLeaked`)

	// Only the host changing src starts a fresh authorized document.
	freshJSON, _ := json.Marshal(frameServer.URL + "/a/app-1?fresh=1")
	run(chromedp.Evaluate[chromedp.Void](`document.getElementById('artifact-frame').src=`+string(freshJSON)),
		chromedp.Poll[chromedp.Void](`window.relayResults.length === 3 || window.relayError`))
	check(`!window.relayError && relayResults.length === 3`)
	assert.EqualValues(t, 9, calls.Load())

	// The exact meta policy narrows the original broad agent header. A second
	// artifact at the same render origin cannot receive connected data in its
	// navigation URL and then use its own broader network policy to leak it.
	evilJSON, _ := json.Marshal(frameServer.URL + "/a/other-app?data=private")
	run(chromedp.Evaluate[chromedp.Void](`document.getElementById('artifact-frame').contentWindow.postMessage({command:'navigate',url:`+string(evilJSON)+`},'*')`),
		chromedp.Poll[chromedp.Void](`cspViolations.includes('frame-src')`))
	assert.Zero(t, evilCalls.Load())
}
