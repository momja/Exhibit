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
	"github.com/momja/Exhibit/internal/connectors"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrivateWidgetWrapperPinsOnePathAndKeepsFreshReloadWrapped(t *testing.T) {
	assert.Contains(t, widgetFramePolicy("https://render.example/w/app-1?t=secret"), "frame-src https://render.example/w/app-1;")
	assert.NotContains(t, widgetFramePolicy("https://render.example/w/app-1?t=secret"), "secret")
	assert.Contains(t, widgetFramePolicy("https://render.example/a/app-1"), "frame-src 'none'")
	assert.Contains(t, widgetFramePolicy("https://render.example/w/.."), "frame-src 'none'")
	assert.NotContains(t, widgetFrameDocument("https://render.example/w/app-1?t=secret"), "<script")
	ro := newTestRouter(t)
	id := createOwnArtifact(t, ro)
	require.Equal(t, http.StatusOK, putWidgetReq(t, ro, id, "<p>Account balance</p>").Code)
	service := connectors.New(connectors.Config{CustomEnabled: true, Store: ro.cfg.Store.(*store.SQLiteStore), Box: ro.cfg.Secrets})
	ro.cfg.ConnectionPolicy = service
	c, err := service.Create(context.Background(), 1, connectors.CreateRequest{Name: "Account", Recipe: "bearer", Origin: "https://api.example.test", Secret: "private-key"}, connectors.UnlimitedLimits())
	require.NoError(t, err)
	_, err = service.Grant(context.Background(), 1, id, connectors.GrantRequest{ConnectionID: c.ID})
	require.NoError(t, err)
	for _, path := range []string{"/", "/partials/card-widget?artifact=" + id, "/agent?artifact=" + id, "/artifacts/" + id + "/edit"} {
		page := getPage(t, ro, path)
		assert.Contains(t, page, "data-widget-wrapper", path)
		assert.Contains(t, page, `sandbox="allow-scripts allow-same-origin"`, path)
	}
	w := doJSON(t, ro, http.MethodGet, "/artifacts/"+id+"/widget/open", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Empty(t, w.Header().Get("Location"))
	assert.Contains(t, w.Header().Get("Content-Security-Policy"), "frame-src http://render.test/w/"+id+"/p/")
	assert.Contains(t, w.Body.String(), "<iframe")
	assert.NotContains(t, w.Body.String(), "<script")
}

func TestRecipientWidgetRefreshKeepsConnectionDataProtected(t *testing.T) {
	in := newGrantedInstance(t)
	ctx := context.Background()
	service := connectors.New(connectors.Config{CustomEnabled: true, Store: in.ro.cfg.Store.(*store.SQLiteStore), Box: in.ro.cfg.Secrets})
	in.ro.cfg.ConnectionPolicy = service
	connection, err := service.Create(ctx, in.ownerTwo, connectors.CreateRequest{Name: "Recipient account", Recipe: "bearer", Origin: "https://api.example.test", Secret: "private-key"}, connectors.UnlimitedLimits())
	require.NoError(t, err)
	_, err = service.Grant(ctx, in.ownerTwo, in.granted, connectors.GrantRequest{ConnectionID: connection.ID})
	require.NoError(t, err)
	page := in.get(t, "/shared", in.cookieTwo)
	require.Equal(t, http.StatusOK, page.Code)
	assert.Contains(t, page.Body.String(), "data-widget-wrapper")
	reload := in.get(t, "/artifacts/"+in.granted+"/widget/viewer/open", in.cookieTwo)
	require.Equal(t, http.StatusOK, reload.Code, reload.Body.String())
	assert.Empty(t, reload.Header().Get("Location"))
	assert.Contains(t, reload.Header().Get("Content-Security-Policy"), "frame-src "+in.ro.cfg.RenderOrigin+"/w/"+in.granted+"/p/")
	assert.Contains(t, reload.Body.String(), "<iframe")
	assert.NotContains(t, reload.Body.String(), "<script")
}

func TestRecipientWidgetRefreshRequiresTrustedFrame(t *testing.T) {
	in := newGrantedInstance(t)
	for _, site := range []string{"cross-site", "same-site", "same-origin"} {
		req := httptest.NewRequest(http.MethodGet, "/artifacts/"+in.granted+"/widget/viewer/open", nil)
		req.AddCookie(in.cookieTwo)
		req.Header.Set("Sec-Fetch-Dest", "iframe")
		req.Header.Set("Sec-Fetch-Site", site)
		w := httptest.NewRecorder()
		in.ro.ServeHTTP(w, req)
		if site == "same-origin" {
			assert.Equal(t, http.StatusFound, w.Code, w.Body.String())
			assert.NotEmpty(t, w.Header().Get("Location"))
		} else {
			assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
			assert.Empty(t, w.Header().Get("Location"))
		}
	}
}

func TestPrivateWidgetBrowserShowsSavedDataAndBlocksSelfNavigation(t *testing.T) {
	ro := newTestRouter(t)
	ctx := context.Background()
	service := connectors.New(connectors.Config{CustomEnabled: true, Store: ro.cfg.Store.(*store.SQLiteStore), Box: ro.cfg.Secrets})
	ro.cfg.ConnectionPolicy = service
	id := createOwnArtifact(t, ro)
	var forbiddenHits, renderHits atomic.Int64
	attacker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		forbiddenHits.Add(1)
		fmt.Fprint(w, "forbidden destination")
	}))
	defer attacker.Close()
	renderServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		renderHits.Add(1)
		if !strings.HasPrefix(r.URL.Path, "/w/"+id) {
			forbiddenHits.Add(1)
		}
		ro.RenderHandler().ServeHTTP(w, r)
	}))
	defer renderServer.Close()
	ro.cfg.RenderOrigin = renderServer.URL
	var view widgetView
	var parentPolicy string
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			assetsHandler().ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Security-Policy", parentPolicy)
		fragment, err := renderPage("cardWidget", view)
		require.NoError(t, err)
		fmt.Fprintf(w, `<!doctype html><html><head><script>window.widgetValue=null;window.widgetReady=false;window.violations=[];addEventListener('message',e=>{if(e.data.testViolation)violations.push(e.data.testViolation);if(e.data.testWidgetValue)widgetValue=e.data.testWidgetValue;if(e.data.__avWidget&&e.data.status==='ready')widgetReady=true;});</script><script src="/assets/gallery/components.js"></script></head><body>%s</body></html>`, fragment)
	}))
	defer app.Close()
	ro.cfg.AppOrigin = app.URL
	appJSON, _ := json.Marshal(app.URL)
	widget := fmt.Sprintf(`<p id="balance"></p><script>const app=%s;const value=localStorage.getItem('balance')||new URL(location.href).searchParams.get('balance');document.getElementById('balance').textContent=value;top.postMessage({testWidgetValue:document.getElementById('balance').textContent},app);addEventListener('message',e=>{if(e.origin!==app)return;if(e.data.command==='navigate'){const target=new URL(e.data.url);target.searchParams.set('balance',value);location.href=target.href;}if(e.data.command==='ready')top.postMessage({__avWidget:true,status:'ready'},app);});</script>`, appJSON)
	require.Equal(t, http.StatusOK, putWidgetReq(t, ro, id, widget).Code)
	require.NoError(t, ro.cfg.Store.SetState(ctx, 1, id, 1, "balance", "PRIVATE-BALANCE-8392"))
	c, err := service.Create(ctx, 1, connectors.CreateRequest{Name: "Account", Recipe: "bearer", Origin: "https://api.example.test", Secret: "private-key"}, connectors.UnlimitedLimits())
	require.NoError(t, err)
	_, err = service.Grant(ctx, 1, id, connectors.GrantRequest{ConnectionID: c.ID})
	require.NoError(t, err)
	a, err := ro.cfg.Store.GetArtifact(ctx, 1, id)
	require.NoError(t, err)
	viewRequest := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(withPrincipal(ctx, Principal{OwnerID: 1, Kind: PrincipalServiceToken}))
	view = newWidgetView(a, ro.renderURLs(viewRequest))
	parentPolicy = ro.renderURLs(viewRequest).framePolicy(id)
	require.True(t, view.Wrapped)
	browser, err := chromium.New(chromiumtest.Start(t))
	require.NoError(t, err)
	browserCtx, cancel, err := browser.Tab(ctx)
	require.NoError(t, err)
	defer cancel()
	browserCtx, timeout := context.WithTimeout(browserCtx, 20*time.Second)
	defer timeout()
	var blocked atomic.Int64
	messages := chromedp.Console(browserCtx)
	go func() {
		for message, err := range messages {
			if err != nil {
				return
			}
			if strings.Contains(message.Text, "frame-src") {
				blocked.Add(1)
			}
		}
	}()
	run := func(actions ...chromedp.Action[chromedp.Void]) {
		for _, action := range actions {
			_, err := chromedp.Run(browserCtx, action)
			require.NoError(t, err)
		}
	}
	run(chromedp.Navigate(app.URL), chromedp.Poll[chromedp.Void](`widgetValue === 'PRIVATE-BALANCE-8392' && widgetReady`))
	run(chromedp.Evaluate[chromedp.Void](`const outer=document.querySelector('.card-widget-frame');outer.addEventListener('__avresolved',()=>window.healthResolved=true);outer.contentDocument.addEventListener('securitypolicyviolation',e=>violations.push(e.violatedDirective));outer.contentDocument.querySelector('iframe').contentWindow.postMessage({command:'ready'},'*');`), chromedp.Poll[chromedp.Void](`window.healthResolved === true`))
	for i, destination := range []string{attacker.URL, renderServer.URL + "/w/other-widget", renderServer.URL + "/w/" + id + "?t=" + ro.tokens.MintAnonymous(id, 1)} {
		if i > 0 {
			run(chromedp.Navigate(app.URL), chromedp.Poll[chromedp.Void](`widgetValue === 'PRIVATE-BALANCE-8392' && widgetReady`))
		}
		encoded, _ := json.Marshal(destination)
		before := blocked.Load()
		beforeRender := renderHits.Load()
		run(chromedp.Evaluate[chromedp.Void](`document.querySelector('.card-widget-frame').contentDocument.querySelector('iframe').contentWindow.postMessage({command:'navigate',url:` + string(encoded) + `},'*');`))
		require.Eventually(t, func() bool { return blocked.Load() > before }, 5*time.Second, 10*time.Millisecond, "browser must report a frame-src refusal; forbidden server requests: %d", forbiddenHits.Load())
		assert.Zero(t, forbiddenHits.Load(), destination)
		assert.Equal(t, beforeRender, renderHits.Load(), "another viewer or anonymous token cannot bypass the private policy path")
	}
}
