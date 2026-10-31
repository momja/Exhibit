package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
	"github.com/momja/Exhibit/internal/auth"
	"github.com/momja/Exhibit/internal/chromium"
	"github.com/momja/Exhibit/internal/chromium/chromiumtest"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/require"
)

// These checks run the shipped templates/modules against native Chromium
// dialogs. Small Node fixtures cover API transactions separately.
type modalBrowser struct {
	t   *testing.T
	ctx context.Context
}

func newModalBrowser(t *testing.T, width int64) modalBrowser {
	t.Helper()
	browser, err := chromium.New(chromiumtest.Start(t))
	require.NoError(t, err)
	ctx, cancel, err := browser.Tab(context.Background())
	require.NoError(t, err)
	t.Cleanup(cancel)
	ctx, timeout := context.WithTimeout(ctx, 60*time.Second)
	t.Cleanup(timeout)
	_, err = chromedp.Call(ctx, emulation.SetDeviceMetricsOverride, emulation.SetDeviceMetricsOverrideParams{Width: width, Height: 800, DeviceScaleFactor: 1})
	require.NoError(t, err)
	return modalBrowser{t, ctx}
}
func (b modalBrowser) run(actions ...chromedp.Action[chromedp.Void]) {
	b.t.Helper()
	require.NoError(b.t, chromedp.Do(b.ctx, actions...))
}
func (b modalBrowser) eval(js string) { b.t.Helper(); b.run(chromedp.Evaluate[chromedp.Void](js)) }
func (b modalBrowser) wait(js string) {
	b.t.Helper()
	require.NoError(b.t, chromedp.Do(b.ctx, chromedp.Poll[chromedp.Void](js, chromedp.WithPollingTimeout(8*time.Second))), js)
}
func (b modalBrowser) check(js string) {
	b.t.Helper()
	ok, err := chromedp.Run(b.ctx, chromedp.Evaluate[bool](js))
	require.NoError(b.t, err)
	require.True(b.t, ok, js)
}
func (b modalBrowser) capture(ticket string, width int64) {
	b.t.Helper()
	if root := os.Getenv("MODAL_SCREENSHOT_DIR"); root != "" {
		dir := filepath.Join(root, ticket)
		require.NoError(b.t, os.MkdirAll(dir, 0755))
		shot, err := chromedp.Run(b.ctx, chromedp.Screenshot(chromedp.CSS(`body`)))
		require.NoError(b.t, err)
		require.NoError(b.t, os.WriteFile(filepath.Join(dir, fmt.Sprintf("%d.png", width)), shot, 0644))
	}
}
func (b modalBrowser) dialogCycle(selector, openJS, focusSelector string) {
	b.t.Helper()
	b.eval(openJS)
	b.wait(`document.querySelector('` + selector + `').open`)
	b.check(`document.activeElement.matches('` + focusSelector + `')`)
	b.eval(`{ const fields=[...document.querySelector('` + selector + `').querySelectorAll('button,input,select,textarea,a[href]')].filter(e=>!e.disabled && e.getClientRects().length && e.tabIndex>=0); fields.at(-1).focus(); }`)
	b.run(chromedp.KeyEvent(kb.Tab))
	b.check(`document.querySelector('` + selector + `').contains(document.activeElement)`)
	b.run(chromedp.KeyEvent(kb.Escape))
	b.wait(`!document.querySelector('` + selector + `').open`)
	b.eval(openJS)
	b.wait(`document.querySelector('` + selector + `').open`)
	b.eval(`document.querySelector('` + selector + `').dispatchEvent(new MouseEvent('click',{bubbles:true,clientX:-1,clientY:-1}))`)
	b.wait(`!document.querySelector('` + selector + `').open`)

	// Every adopted consumer must release native modality on removal and
	// reconnect the same node without retaining document-level listeners.
	b.eval(openJS)
	b.wait(`document.querySelector('` + selector + `').open`)
	b.eval(`window.detachedModal=document.querySelector('` + selector + `');window.detachedParent=detachedModal.parentElement;detachedModal.remove()`)
	b.wait(`!detachedModal.open`)
	b.eval(`detachedParent.append(detachedModal)`)
	b.run(chromedp.Sleep(30 * time.Millisecond))
	b.eval(openJS)
	b.wait(`detachedModal.open`)
	b.run(chromedp.KeyEvent(kb.Escape))
	b.wait(`!detachedModal.open`)

	// Drain the real frame's message queue before a transaction test resets its
	// reply log. Earlier cancellation replies must not race that reset.
	b.eval(`if(window.permissionReport){window.permissionPong=false;document.querySelector('iframe').contentWindow.postMessage({__modalTestPing:true},'*')}`)
	b.wait(`!window.permissionReport || window.permissionPong`)
}

func TestTagModalBrowser(t *testing.T) {
	for _, width := range []int64{1280, 390} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			ro := newTestRouter(t)
			id := createTestArtifact(t, ro, "Tag modal sample")
			tag := createTestTag(t, ro, "Charts", "#0088ff")
			require.Equal(t, 204, doJSON(t, ro, "POST", "/api/tags/"+tag.ID+"/artifacts/"+id, nil).Code)
			app := httptest.NewServer(ro)
			defer app.Close()
			ro.cfg.AppOrigin = app.URL
			b := newModalBrowser(t, width)
			b.run(chromedp.Navigate(app.URL + "/artifacts/" + id + "/edit"))
			b.wait(`document.querySelector('#tags-panel [data-action="edit-tag"]') && document.querySelector('.cm-editor')`)
			b.eval(`document.getElementById('tags-panel').open=true`)
			open := `document.querySelector('#tags-panel [data-action="edit-tag"]').click()`
			b.dialogCycle("#tag-edit-modal", open, "#tag-edit-name")
			b.check(`document.activeElement.matches('[data-action="edit-tag"]')`)
			b.eval(open)
			checkModalVisuals(t, b.ctx, "#tag-edit-modal")
			b.capture("Exh-p3ah", width)
			b.eval(`document.getElementById('tag-edit-name').value='';document.getElementById('tag-edit-save').click()`)
			b.check(`document.querySelector('#tag-edit-modal').open && document.getElementById('tag-edit-error').textContent==='Name is required.'`)
			b.eval(`document.getElementById('tag-edit-name').value='Graphs'; document.getElementById('tag-edit-save').click()`)
			b.wait(`!document.getElementById('tag-edit-modal').open && document.querySelector('[data-tag-name="Graphs"]') && document.activeElement.dataset.tagName==='Graphs'`)
			// A pending mutation blocks every dismissal and another submission.
			// Failure keeps the working copy and restores the existing buttons.
			b.eval(open)
			b.eval(`window.tagAPI=window.apiFetch; window.tagCalls=0; window.apiFetch=(path,options)=> {
 if (path.startsWith('/api/tags/') && options.method==='PATCH') {
  tagCalls++; return new Promise(resolve=>window.finishTag=()=>resolve({ok:false,json:async()=>({error:'Please retry'})}));
 } return tagAPI(path,options);
}; document.getElementById('tag-edit-name').value='Plots';document.getElementById('tag-edit-save').click()`)
			b.wait(`tagCalls===1 && document.getElementById('tag-edit-save').disabled`)
			b.run(chromedp.KeyEvent(kb.Escape))
			b.eval(`document.getElementById('tag-edit-modal').dispatchEvent(new MouseEvent('click',{bubbles:true,clientX:-1,clientY:-1}));document.getElementById('tag-edit-save').click();document.getElementById('tag-edit-cancel').click()`)
			b.check(`tagCalls===1 && document.getElementById('tag-edit-modal').open`)
			b.eval(`finishTag()`)
			b.wait(`document.getElementById('tag-edit-error').textContent==='Please retry' && !document.getElementById('tag-edit-save').disabled`)
			b.check(`document.getElementById('tag-edit-name').value==='Plots' && document.getElementById('tag-edit-modal').open`)
			b.eval(`window.apiFetch=tagAPI;document.getElementById('tag-edit-save').click()`)
			b.wait(`!document.getElementById('tag-edit-modal').open && document.activeElement.dataset.tagName==='Plots'`)
			// Removing/reconnecting the native dialog releases the top layer and listeners.
			b.eval(open)
			b.eval(`window.savedDialog=document.getElementById('tag-edit-modal');window.savedParent=savedDialog.parentElement;savedDialog.remove()`)
			b.wait(`!savedDialog.open`)
			b.eval(`savedParent.append(savedDialog)`)
			b.run(chromedp.Sleep(20 * time.Millisecond))
			b.eval(open)
			b.wait(`savedDialog.open`)
			b.run(chromedp.KeyEvent(kb.Escape))
			b.wait(`!savedDialog.open`)
		})
	}
}

func TestShareModalBrowser(t *testing.T) {
	for _, width := range []int64{1280, 390} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			ro := newTestRouter(t)
			id := createTestArtifact(t, ro, "Sharing sample")
			app := httptest.NewServer(ro)
			defer app.Close()
			ro.cfg.AppOrigin = app.URL
			b := newModalBrowser(t, width)
			b.run(chromedp.Navigate(app.URL + "/artifacts/" + id))
			b.wait(`document.getElementById('share-open')`)
			open := `document.getElementById('share-open').click()`
			b.dialogCycle("#share-panel", open, "#share-add-input")
			b.check(`document.activeElement.id==='share-open'`)
			b.eval(open)
			checkModalVisuals(t, b.ctx, "#share-panel")
			b.capture("Exh-9xfp", width)
			b.eval(`document.getElementById('share-add-input').value='unfinished';document.getElementById('share-add-input').dispatchEvent(new Event('input',{bubbles:true})); document.getElementById('state-how-it-works').open=true;document.getElementById('share-state-shared').click()`)
			b.wait(`document.getElementById('share-state-mode').dataset.savedMode==='shared' && !document.getElementById('share-close').disabled`)
			b.check(`document.getElementById('share-panel').open && document.getElementById('share-add-input').value==='unfinished' && document.getElementById('state-how-it-works').open`)
			b.check(`document.getElementById('share-panel').contains(document.activeElement)`)
			b.eval(`document.getElementById('share-close').click()`)
			b.wait(`!document.getElementById('share-panel').open`)
			// Short-window content, including Done, remains reachable by scrolling.
			_, err := chromedp.Call(b.ctx, emulation.SetDeviceMetricsOverride, emulation.SetDeviceMetricsOverrideParams{Width: width, Height: 400, DeviceScaleFactor: 1})
			require.NoError(t, err)
			b.eval(open)
			b.eval(`document.getElementById('share-close').scrollIntoView()`)
			b.check(`document.getElementById('share-close').getBoundingClientRect().bottom<=400 && document.getElementById('share-add-input').value===''`)
			// Real guest rows and explanatory copy must not bury the exit or
			// overflow the viewport after typography changes.
			var names []string
			for n := 0; n < 12; n++ {
				name := fmt.Sprintf("long-recipient-%02d-abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz", n)
				seedLocalAccount(t, ro, name)
				names = append(names, name)
			}
			grantTo(t, ro, id, names)
			b.eval(`document.body.dispatchEvent(new Event('exhibit:shares-changed'))`)
			b.wait(`document.querySelectorAll('.share-grant').length===12`)
			b.eval(`document.getElementById('state-how-it-works').open=true; document.getElementById('share-panel').scrollTop=0`)
			b.check(`document.querySelector('.share-people-list').clientHeight<=120 && document.querySelector('.share-people-list').scrollHeight>document.querySelector('.share-people-list').clientHeight`)
			checkModalVisuals(t, b.ctx, "#share-panel")
			b.capture("Exh-9xfp/short-content", width)
			b.eval(`document.getElementById('share-status').textContent='Changes saved. The people listed above now have access to this artifact and its saved data.';document.getElementById('share-close').scrollIntoView()`)
			b.check(`document.getElementById('share-close').getBoundingClientRect().bottom<=400 && document.getElementById('share-panel').scrollWidth===document.getElementById('share-panel').clientWidth`)
			b.capture("Exh-9xfp/short-actions", width)
			b.eval(`document.getElementById('share-close').click()`)
			b.check(`!document.getElementById('share-panel').open`)

		})
	}
}

func TestAgentKeyModalBrowser(t *testing.T) {
	for _, width := range []int64{1280, 390} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			ro := newTestRouter(t)
			ro.cfg.MockEnabled = true
			app := httptest.NewServer(ro)
			defer app.Close()
			ro.cfg.AppOrigin = app.URL
			b := newModalBrowser(t, width)
			b.run(chromedp.Navigate(app.URL + "/agent"))
			b.wait(`document.getElementById('key-modal').open`)
			b.dialogCycle("#key-modal", `document.getElementById('key-btn').click()`, "#key-secret")
			b.eval(`document.getElementById('key-btn').click()`)
			checkModalVisuals(t, b.ctx, "#key-modal")
			b.capture("Exh-uxyy", width)
			// Delay at the transport boundary to exercise real native dismissal guards.
			b.eval(`window.savedFetch=window.fetch;window.fetch=(url,options)=>options?.method==='PUT'?new Promise(resolve=>window.finishKey=()=>resolve(new Response('{"error":"Provider unavailable"}',{status:503,headers:{'Content-Type':'application/json'}}))):savedFetch(url,options);document.getElementById('key-secret').value='fixture';document.getElementById('key-save').click()`)
			b.wait(`document.getElementById('key-cancel').disabled`)
			b.check(`document.getElementById('key-provider').matches(':disabled') && document.getElementById('key-secret').matches(':disabled')`)
			b.run(chromedp.KeyEvent(kb.Escape))
			b.check(`document.getElementById('key-modal').open`)
			b.eval(`finishKey()`)
			b.wait(`document.getElementById('key-error').textContent==='Provider unavailable' && !document.getElementById('key-cancel').disabled`)
			b.eval(`document.getElementById('key-cancel').click()`)
			b.wait(`!document.getElementById('key-modal').open`)
			b.check(`document.getElementById('key-secret').value==='' && document.activeElement.id==='key-btn'`)
		})
	}
}

func TestFilterModalBrowser(t *testing.T) {
	for _, width := range []int64{1280, 390} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			mux := http.NewServeMux()
			mux.Handle("/assets/", assetsHandler())
			mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, `<!doctype html><link rel="stylesheet" href="/assets/gallery/tokens.css"><link rel="stylesheet" href="/assets/gallery/components.css"><link rel="stylesheet" href="/assets/gallery/index.css"><link rel="stylesheet" href="/assets/phosphor/regular.css"><body>`)
				for _, id := range []string{"one", "two"} {
					sheet, err := renderPage("galleryFilterSheet", galleryGridData{ID: id, Filters: galleryFilters{Sections: []galleryFilterSection{{Label: "Collections", Options: []galleryFilterOption{{Label: "Research", PageURL: "#research"}}}, {Label: "Tags", Options: []galleryFilterOption{{Label: "Work", Color: "#0088ff", Active: true, PageURL: "#work"}}}}}})
					require.NoError(t, err)
					fmt.Fprintf(w, `<section data-controller="filter-sheet"><button id="%s-trigger" data-filter-sheet-target="trigger" data-action="filter-sheet#open">Filters</button>%s</section>`, id, sheet)
				}
				fmt.Fprint(w, `<script type="module" src="/assets/gallery/controllers.js"></script></body>`)
			})
			app := httptest.NewServer(mux)
			defer app.Close()
			b := newModalBrowser(t, width)
			b.run(chromedp.Navigate(app.URL))
			b.run(chromedp.Sleep(50 * time.Millisecond))
			b.dialogCycle("#one-filter-sheet", `document.getElementById('one-trigger').click()`, `[aria-label="Close filters"]`)
			b.check(`document.activeElement.id==='one-trigger'`)
			b.eval(`document.getElementById('two-trigger').click()`)
			b.wait(`document.getElementById('two-filter-sheet').open`)
			checkModalVisuals(t, b.ctx, "#two-filter-sheet")
			b.capture("Exh-gynk/instances", width)
			b.run(chromedp.KeyEvent(kb.Escape))
			b.check(`document.activeElement.id==='two-trigger'`)
		})
	}
}

func TestAdminEntitlementModalBrowser(t *testing.T) {
	for _, width := range []int64{1280, 390} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			in := newAdminInstance(t)
			longName := strings.Repeat("long-account-", 10)
			longAccount, err := in.ro.cfg.Store.CreateLocalUser(context.Background(), store.NewLocalUser{ExternalID: auth.LocalExternalID(longName), Email: longName + "@example.test", PasswordHash: "x"})
			require.NoError(t, err)
			app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { r.AddCookie(in.adminCookie); in.ro.ServeHTTP(w, r) }))
			defer app.Close()
			in.ro.cfg.AppOrigin = app.URL
			b := newModalBrowser(t, width)
			b.run(chromedp.Navigate(app.URL + "/admin/users"))
			b.wait(`document.querySelector('[data-action="entitlement"]')`)
			open := `document.querySelector('[data-action="entitlement"]').click()`
			b.dialogCycle("#entitlement-dialog", open, "#entitlement-plan")
			b.check(`document.activeElement.dataset.action==='entitlement'`)
			b.eval(open)
			checkModalVisuals(t, b.ctx, "#entitlement-dialog")
			b.check(`document.querySelectorAll("#entitlement-dialog-title").length===1 && document.querySelector("#entitlement-dialog").getAttribute("aria-labelledby")==="entitlement-dialog-title"`)
			b.capture("Exh-b98w", width)
			for _, bad := range []string{"12e", "-", "5,000", "1.5", "9007199254740992"} {
				b.eval(fmt.Sprintf(`document.getElementById('entitlement-storage').value=%q;document.getElementById('entitlement-save').click()`, bad))
				b.check(`document.getElementById('entitlement-dialog').open && !document.getElementById('entitlement-error').hidden`)
			}
			b.eval(`window.savedFetch=window.fetch;window.fetch=(url,opts)=>opts?.method==='PATCH'?new Promise(resolve=>window.finishEntitlement=()=>resolve(new Response('Not authorized',{status:403}))):savedFetch(url,opts);document.getElementById('entitlement-storage').value='';document.getElementById('entitlement-save').click()`)
			b.wait(`document.getElementById('entitlement-cancel').disabled`)
			b.check(`document.getElementById('entitlement-plan').matches(':disabled') && document.getElementById('entitlement-storage').matches(':disabled')`)
			b.run(chromedp.KeyEvent(kb.Escape))
			b.check(`document.getElementById('entitlement-dialog').open`)
			b.eval(`finishEntitlement()`)
			b.wait(`document.getElementById('entitlement-error').textContent==='Not authorized'`)
			b.check(`document.getElementById('entitlement-dialog').open && document.getElementById('entitlement-storage').value===''`)
			b.eval(fmt.Sprintf(`window.fetch=savedFetch;document.getElementById('entitlement-cancel').click();document.querySelector('[data-action="entitlement"][data-user-id="%d"]').click()`, longAccount.ID))
			checkModalVisuals(t, b.ctx, "#entitlement-dialog")
			b.check(`document.getElementById('entitlement-dialog-title').scrollWidth<=document.getElementById('entitlement-dialog-title').clientWidth`)
			b.capture("Exh-b98w/long-account", width)

		})
	}
}

func TestEditorLeaveModalBrowser(t *testing.T) {
	for _, width := range []int64{1280, 390} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			ro := newTestRouter(t)
			id := createTestArtifact(t, ro, "Unsaved sample")
			app := httptest.NewServer(ro)
			defer app.Close()
			ro.cfg.AppOrigin = app.URL
			b := newModalBrowser(t, width)
			b.run(chromedp.Navigate(app.URL + "/artifacts/" + id + "/edit"))
			b.wait(`document.querySelector('.cm-editor')`)
			b.eval(`document.getElementById('title').value='Draft';document.getElementById('title').dispatchEvent(new Event('input',{bubbles:true}));document.querySelector('.app-header-back').click()`)
			b.wait(`document.querySelector('.editor-leave-dialog').open`)
			checkModalVisuals(t, b.ctx, ".editor-leave-dialog")
			b.capture("Exh-yrgf", width)
			if width == 1280 {
				b.check(`(() => { const buttons=[...document.querySelectorAll('.editor-leave-dialog .btn')];const needed=buttons.reduce((sum,b)=>sum+b.getBoundingClientRect().width,16);return needed>420-46 && needed<=480-46 && buttons.every(b=>Math.abs(b.getBoundingClientRect().top-buttons[0].getBoundingClientRect().top)<=2); })()`)
			}
			_, err := chromedp.Call(b.ctx, emulation.SetDeviceMetricsOverride, emulation.SetDeviceMetricsOverrideParams{Width: width, Height: 240, DeviceScaleFactor: 1})
			require.NoError(t, err)
			b.eval(`document.querySelector('.editor-leave-dialog .btn:last-child').scrollIntoView()`)
			b.check(`[...document.querySelectorAll('.editor-leave-dialog .btn')].every(b=>b.getBoundingClientRect().bottom<=240)`)
			b.capture("Exh-yrgf/short-actions", width)
			_, err = chromedp.Call(b.ctx, emulation.SetDeviceMetricsOverride, emulation.SetDeviceMetricsOverrideParams{Width: width, Height: 800, DeviceScaleFactor: 1})
			require.NoError(t, err)

			b.check(`document.activeElement.hasAttribute('data-keep-editing')`)
			// This coordinator intentionally requires a decision on backdrop clicks.
			b.eval(`document.querySelector('.editor-leave-dialog').dispatchEvent(new MouseEvent('click',{bubbles:true,clientX:-1,clientY:-1}))`)
			b.check(`document.querySelector('.editor-leave-dialog').open`)
			b.run(chromedp.KeyEvent(kb.Escape))
			b.wait(`!document.querySelector('.editor-leave-dialog').open`)
			b.check(`document.activeElement.matches('.app-header-back') && document.getElementById('title').value==='Draft'`)
			b.eval(`document.querySelector('.app-header-back').click();window.editorDialog=document.querySelector('.editor-leave-dialog');window.editorDialogParent=editorDialog.parentElement;editorDialog.remove()`)
			b.wait(`!editorDialog.open`)
			b.eval(`editorDialogParent.append(editorDialog)`)
			b.run(chromedp.Sleep(30 * time.Millisecond))
			b.eval(`document.querySelector('.app-header-back').click()`)
			b.wait(`editorDialog.open`)
			b.run(chromedp.KeyEvent(kb.Escape))
			b.wait(`!editorDialog.open`)

		})
	}
}

// Real viewer bootstrap for the permission consumers. Messages retain the
// current sandbox frame identity; only transport delays are stubbed per case.
func newPermissionViewer(t *testing.T, width int64, agent bool) (modalBrowser, string) {
	t.Helper()
	ro := newTestRouter(t)
	id := createArtifact(t, ro, map[string]any{"title": "Permission sample", "body": `<!doctype html><h1>Permission sample</h1><script>addEventListener('message',event=>{if(event.data.__modalTestPing)parent.postMessage({__modalTestPong:true},'*');if(event.data.__avClipboardResult || event.data.__avMediaResult || event.data.__avGeolocationResult)parent.postMessage({__modalTestReply:event.data},'*')})</script>`})
	app := httptest.NewServer(ro)
	t.Cleanup(app.Close)
	ro.cfg.AppOrigin = app.URL
	render := httptest.NewServer(ro.RenderHandler())
	t.Cleanup(render.Close)
	ro.cfg.RenderOrigin = render.URL
	b := newModalBrowser(t, width)
	path := "/artifacts/" + id
	if agent {
		path = "/agent?artifact=" + id
	}
	b.run(chromedp.Navigate(app.URL + path))
	b.wait(`document.querySelector('iframe')`)
	if agent {
		b.wait(`document.getElementById('key-modal').open`)
		b.eval(`document.getElementById('key-cancel').click();document.getElementById('tab-preview').click()`)
	}
	b.eval(fmt.Sprintf(`window.permissionReplies=[];addEventListener('message',event=>{if(event.source===document.querySelector('iframe').contentWindow){if(event.data.__modalTestReply)permissionReplies.push(event.data.__modalTestReply);if(event.data.__modalTestPong)window.permissionPong=true}});window.permissionReport=data=>window.dispatchEvent(new MessageEvent('message',{source:document.querySelector('iframe').contentWindow,data:{artifactId:%q,...data}}))`, id))
	return b, id
}

func TestNetworkModalBrowser(t *testing.T) {
	for _, agent := range []bool{false, true} {
		for _, width := range []int64{1280, 390} {
			t.Run(fmt.Sprintf("agent=%t/%d", agent, width), func(t *testing.T) {
				b, _ := newPermissionViewer(t, width, agent)
				report := `permissionReport({__avNetwork:true,origin:'https://first.example.test',directive:'connect-src'})`
				b.dialogCycle("#net-modal", report, "#net-once")
				b.eval(report)
				checkModalVisuals(t, b.ctx, "#net-modal")
				b.capture("Exh-1grc", width)
				b.eval(`permissionReport({__avNetwork:true,origin:'https://second.example.test'});window.savedFetch=window.fetch;window.fetch=(url,opts)=>opts?.method==='POST'?new Promise(resolve=>window.finishNetwork=()=>resolve(new Response('{}',{status:200}))):savedFetch(url,opts);document.getElementById('net-never').click()`)
				b.wait(`document.getElementById('net-never').disabled`)
				b.run(chromedp.KeyEvent(kb.Escape))
				b.wait(`document.getElementById('net-modal').open && document.getElementById('net-origin').textContent==='https://second.example.test'`)
				b.eval(`finishNetwork()`)
				b.run(chromedp.Sleep(30 * time.Millisecond))
				b.check(`document.getElementById('net-modal').open && document.getElementById('net-origin').textContent==='https://second.example.test' && document.activeElement.id==='net-once'`)
				b.run(chromedp.KeyEvent(kb.Escape))
				b.wait(`!document.getElementById('net-modal').open`)
			})
		}
	}
}

func TestDownloadModalBrowser(t *testing.T) {
	for _, width := range []int64{1280, 390} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			b, _ := newPermissionViewer(t, width, false)
			report := `permissionReport({__avDownload:true,filename:'report.csv',mime:'text/csv',bytes:new ArrayBuffer(3)})`
			b.dialogCycle("#dl-modal", report, "#dl-block")
			b.eval(report)
			checkModalVisuals(t, b.ctx, "#dl-modal")
			b.capture("Exh-fvzb", width)
			// A late grant can persist, but cannot download discarded or replaced bytes.
			b.eval(`window.downloads=[];HTMLAnchorElement.prototype.click=function(){downloads.push(this.download)};window.savedFetch=window.fetch;window.fetch=(url,opts)=>opts?.method==='PATCH'?new Promise(resolve=>window.finishDownload=(ok=true)=>resolve(new Response('{}',{status:ok?200:503}))):savedFetch(url,opts);document.getElementById('dl-allow').click()`)
			b.wait(`document.getElementById('dl-allow').disabled`)
			b.eval(`finishDownload(false)`)
			b.wait(`!document.getElementById('dl-allow').disabled`)
			b.check(`document.querySelector('#dl-modal [data-permission-status]').textContent.includes('download') && document.getElementById('dl-modal').open`)
			b.eval(`document.getElementById('dl-allow').click()`)
			b.wait(`document.getElementById('dl-allow').disabled`)
			b.run(chromedp.KeyEvent(kb.Escape))
			b.eval(`finishDownload()`)
			b.run(chromedp.Sleep(30 * time.Millisecond))
			b.check(`downloads.length===0 && !document.getElementById('dl-modal').open`)
			b.eval(`window.fetch=savedFetch;window.savedObjectURL=URL.createObjectURL;URL.createObjectURL=blob=>{window.downloadBlob=blob;return savedObjectURL(blob)}`)
			b.eval(report)
			b.wait(`document.getElementById('dl-modal').open`)
			b.eval(`document.getElementById('dl-allow').click()`)
			b.wait(`downloads.length===1`)
			b.check(`downloads[0]==='report.csv' && downloadBlob.size===3 && downloadBlob.type==='text/csv' && !document.getElementById('dl-modal').open`)

		})
	}
}

func TestClipboardModalBrowser(t *testing.T) {
	for _, width := range []int64{1280, 390} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			b, _ := newPermissionViewer(t, width, false)
			report := `permissionReport({__avClipboard:true,id:'first',op:'read'})`
			b.dialogCycle("#clip-modal", report, "#clip-block")
			b.eval(report)
			checkModalVisuals(t, b.ctx, "#clip-modal")
			b.capture("Exh-3917", width)
			b.eval(`window.clipGestures=[];Object.defineProperty(navigator,'clipboard',{value:{readText(){clipGestures.push(navigator.userActivation.isActive);return Promise.resolve('test clipboard')},writeText(){clipGestures.push(navigator.userActivation.isActive);return Promise.resolve()}}});window.savedFetch=window.fetch;window.fetch=(url,opts)=>opts?.method==='PATCH'?new Promise(resolve=>window.finishClip=()=>resolve(new Response('{}',{status:200}))):savedFetch(url,opts);permissionReplies=[]`)
			b.run(chromedp.Click(chromedp.CSS(`#clip-allow`)))
			b.wait(`document.getElementById('clip-allow').disabled`)
			b.check(`clipGestures.length===1 && clipGestures[0]`)
			// Displacement denies the first request once; its late grant cannot answer second.
			b.eval(`permissionReport({__avClipboard:true,id:'second',op:'write',text:'next'})`)
			b.wait(`permissionReplies.length===1`)
			b.check(`permissionReplies[0].id==='first' && !permissionReplies[0].ok`)
			b.eval(`finishClip()`)
			b.run(chromedp.Sleep(30 * time.Millisecond))
			b.check(`permissionReplies.length===1 && document.getElementById('clip-modal').open && document.activeElement.id==='clip-block'`)
			b.run(chromedp.KeyEvent(kb.Escape))
			b.wait(`permissionReplies.length===2`)
			b.check(`permissionReplies[1].id==='second' && !permissionReplies[1].ok`)
			b.eval(`window.fetch=savedFetch;permissionReport({__avClipboard:true,id:'success',op:'read'})`)
			b.run(chromedp.Click(chromedp.CSS(`#clip-allow`)))
			b.wait(`permissionReplies.length===3`)
			b.check(`permissionReplies[2].id==='success' && permissionReplies[2].ok && permissionReplies[2].text==='test clipboard' && clipGestures.at(-1) && !document.getElementById('clip-modal').open`)
		})
	}
}

func TestLinkModalBrowser(t *testing.T) {
	for _, width := range []int64{1280, 390} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			b, _ := newPermissionViewer(t, width, false)
			report := `permissionReport({__avNavigate:true,url:'https://example.test/report'})`
			b.dialogCycle("#link-modal", report, "#link-block")
			b.eval(report)
			checkModalVisuals(t, b.ctx, "#link-modal")
			b.capture("Exh-d0np", width)
			b.eval(`window.tabs=[];window.open=()=>{const tab={opener:window,location:null,closed:false,close(){this.closed=true}};tabs.push(tab);return tab};window.savedFetch=window.fetch;window.fetch=(url,opts)=>opts?.method==='PATCH'?new Promise(resolve=>window.finishLink=(ok=true)=>resolve(new Response('{}',{status:ok?200:503}))):savedFetch(url,opts)`)
			b.run(chromedp.Click(chromedp.CSS(`#link-allow`)))
			b.wait(`document.getElementById('link-block').disabled`)
			b.run(chromedp.KeyEvent(kb.Escape))
			b.check(`document.getElementById('link-modal').open && tabs.length===1 && tabs[0].opener===null`)
			b.eval(`permissionReport({__avNavigate:true,url:'https://other.example.test/'})`)
			b.check(`document.getElementById('link-host').textContent==='example.test'`)
			b.eval(`finishLink(false)`)
			b.wait(`!document.getElementById('link-block').disabled`)
			b.check(`tabs[0].closed && document.getElementById('link-modal').open && document.querySelector('#link-modal [data-permission-status]').textContent.includes('link')`)
			b.run(chromedp.Click(chromedp.CSS(`#link-allow`)))
			b.wait(`tabs.length===2`)
			b.eval(`finishLink()`)
			b.wait(`!document.getElementById('link-modal').open`)
			b.check(`tabs[1].location==='https://example.test/report' && document.activeElement===document.querySelector('iframe')`)
		})
	}
}

func TestMediaModalBrowser(t *testing.T) {
	for _, width := range []int64{1280, 390} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			b, _ := newPermissionViewer(t, width, false)
			report := `permissionReport({__avMedia:true,id:'camera',video:true,audio:false})`
			b.dialogCycle("#media-modal", report, "#media-block")
			b.eval(report)
			checkModalVisuals(t, b.ctx, "#media-modal")
			b.capture("Exh-w432", width)
			b.run(chromedp.Click(chromedp.CSS(`#media-allow`)))
			b.wait(`document.getElementById('direct-open-modal').open`)
			b.run(chromedp.KeyEvent(kb.Escape))
			b.wait(`!document.getElementById('direct-open-modal').open`)
			b.check(`document.getElementById('media-modal').open && document.activeElement.id==='media-allow'`)
			b.eval(`window.tabs=[];window.writes=[];window.open=()=>{const tab={opener:window,location:null,closed:false,close(){this.closed=true}};tabs.push(tab);return tab};window.savedFetch=window.fetch;window.fetch=(url,opts)=>{if(opts?.method==='PATCH')writes.push(JSON.parse(opts.body));return savedFetch(url,opts)};permissionReplies=[]`)
			b.run(chromedp.Click(chromedp.CSS(`#media-allow`)))
			b.wait(`document.getElementById('direct-open-modal').open`)
			b.run(chromedp.Click(chromedp.CSS(`#direct-open-confirm`)))
			b.wait(`permissionReplies.length===1`)
			b.check(`writes.length===1 && writes[0].camera_approved===true && permissionReplies[0].id==='camera' && permissionReplies[0].name==='NotSupportedError' && tabs[0].opener===null && !document.getElementById('media-modal').open`)
			b.eval(`permissionReport({__avMedia:true,id:'mic',audio:true,video:false})`)
			b.wait(`document.getElementById('media-modal').open`)
			b.check(`document.getElementById('media-devices').textContent==='your microphone' && document.getElementById('media-icon').classList.contains('ph-microphone')`)
			b.run(chromedp.KeyEvent(kb.Escape))
			b.wait(`permissionReplies.length===2`)
			b.check(`permissionReplies[1].id==='mic' && permissionReplies[1].name==='NotAllowedError'`)
		})
	}
}

func TestLocationModalBrowser(t *testing.T) {
	for _, width := range []int64{1280, 390} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			b, _ := newPermissionViewer(t, width, false)
			report := `permissionReport({__avGeolocation:true,id:1})`
			b.dialogCycle("#geo-modal", report, "#geo-block")
			b.eval(report)
			checkModalVisuals(t, b.ctx, "#geo-modal")
			b.capture("Exh-7hh4", width)
			b.run(chromedp.Click(chromedp.CSS(`#geo-allow`)))
			b.wait(`document.getElementById('direct-open-modal').open`)
			b.run(chromedp.KeyEvent(kb.Escape))
			b.wait(`!document.getElementById('direct-open-modal').open`)
			b.check(`document.getElementById('geo-modal').open && document.activeElement.id==='geo-allow'`)
			b.eval(`permissionReplies=[];window.tabs=[];window.open=()=>{const tab={opener:window,location:null,closed:false,close(){this.closed=true}};tabs.push(tab);return tab};permissionReport({__avGeolocation:true,id:2})`)
			b.wait(`permissionReplies.length===1`)
			b.check(`permissionReplies[0].id===1 && permissionReplies[0].error==='User denied Geolocation'`)
			b.run(chromedp.Click(chromedp.CSS(`#geo-allow`)))
			b.wait(`document.getElementById('direct-open-modal').open`)
			b.run(chromedp.Click(chromedp.CSS(`#direct-open-confirm`)))
			b.wait(`permissionReplies.length===2`)
			b.check(`permissionReplies[1].id===2 && !permissionReplies[1].banner && tabs[0].opener===null && !document.getElementById('geo-modal').open`)
		})
	}
}

func TestDirectOpenModalBrowser(t *testing.T) {
	for _, agent := range []bool{false, true} {
		for _, width := range []int64{1280, 390} {
			t.Run(fmt.Sprintf("agent=%t/%d", agent, width), func(t *testing.T) {
				b, _ := newPermissionViewer(t, width, agent)
				trigger := "#direct-open-toolbar"
				if agent {
					trigger = "#pv-open"
				}
				open := `document.querySelector('` + trigger + `').click()`
				b.dialogCycle("#direct-open-modal", open, "#direct-open-cancel")
				b.eval(open)
				b.capture("Exh-sqny", width)
				b.check(`document.documentElement.scrollWidth<=innerWidth && !document.querySelector('body > [inert]')`)
				// Explicit content clicks keep the dialog open; only outside bounds dismiss.
				b.eval(`document.getElementById('direct-open-title').click()`)
				b.check(`document.getElementById('direct-open-modal').open`)
				b.eval(`window.tabs=[];window.blockPopup=true;window.open=()=>{if(blockPopup)return null;const tab={opener:window,closed:false,location:null,close(){this.closed=true}};tabs.push(tab);return tab}`)
				b.run(chromedp.Click(chromedp.CSS(`#direct-open-confirm`)))
				b.wait(`document.getElementById('direct-open-status').textContent.includes('blocked')`)
				b.check(`document.getElementById('direct-open-modal').open`)
				b.eval(`blockPopup=false;document.getElementById('direct-open-remember').checked=true`)
				b.run(chromedp.Click(chromedp.CSS(`#direct-open-confirm`)))
				b.wait(`!document.getElementById('direct-open-modal').open`)
				b.check(`tabs.length===1 && tabs[0].opener===null && tabs[0].location.includes('/open')`)
				b.eval(open)
				b.wait(`tabs.length===2`)
				b.check(`!document.getElementById('direct-open-modal').open`)
			})
		}
	}
}

func TestPermissionModalStackBrowser(t *testing.T) {
	b, _ := newPermissionViewer(t, 390, false)
	b.eval(`permissionReport({__avDownload:true,filename:'queued.txt',bytes:new ArrayBuffer(1)});permissionReport({__avClipboard:true,id:'top',op:'read'})`)
	b.wait(`document.getElementById('dl-modal').open && document.getElementById('clip-modal').open`)
	b.run(chromedp.KeyEvent(kb.Escape))
	b.wait(`!document.getElementById('clip-modal').open`)
	b.check(`document.getElementById('dl-modal').open && document.getElementById('dl-modal').contains(document.activeElement)`)
	b.run(chromedp.KeyEvent(kb.Escape))
	b.wait(`!document.getElementById('dl-modal').open`)
	b.check(`document.activeElement===document.querySelector('iframe')`)
	// Native nested confirmation cancels only itself, then the capability.
	b.eval(`permissionReport({__avMedia:true,id:'nested',video:true})`)
	b.run(chromedp.Click(chromedp.CSS(`#media-allow`)))
	b.wait(`document.getElementById('direct-open-modal').open`)
	b.run(chromedp.KeyEvent(kb.Escape))
	b.check(`!document.getElementById('direct-open-modal').open && document.getElementById('media-modal').open && document.activeElement.id==='media-allow'`)
	b.run(chromedp.KeyEvent(kb.Escape))
	b.wait(`!document.getElementById('media-modal').open`)
}

func TestDirectOpenPendingBrowser(t *testing.T) {
	b, _ := newPermissionViewer(t, 390, false)
	b.eval(`window.directResults=[];window.tabs=[];window.open=()=>{const tab={opener:window,location:null,closed:false,close(){this.closed=true}};tabs.push(tab);return tab};document.dispatchEvent(new CustomEvent('exhibit:open-directly',{detail:{trigger:document.getElementById('pip-more'),approve:()=>new Promise(resolve=>window.approveDirect=resolve),resolve:value=>directResults.push(value)}}))`)
	b.wait(`document.getElementById('direct-open-modal').open`)
	b.eval(`document.getElementById('direct-open-remember').checked=true`)
	b.run(chromedp.Click(chromedp.CSS(`#direct-open-confirm`)))
	b.wait(`document.getElementById('direct-open-confirm').disabled`)
	b.run(chromedp.KeyEvent(kb.Escape))
	b.check(`directResults.length===1 && directResults[0]===false && tabs[0].closed`)
	b.eval(`approveDirect(true)`)
	b.run(chromedp.Sleep(30 * time.Millisecond))
	b.check(`directResults.length===1 && tabs[0].location===null && !document.getElementById('direct-open-modal').open && !Object.keys(localStorage).some(key=>key.startsWith('exhibit:direct-open:'))`)
}

func TestDirectOpenPreviewSwapBrowser(t *testing.T) {
	b, _ := newPermissionViewer(t, 390, true)
	b.eval(`window.oldOpen=document.getElementById('pv-open');oldOpen.click();document.body.dispatchEvent(new CustomEvent('exhibit:artifact-saved'))`)
	b.wait(`!oldOpen.isConnected && document.getElementById('pv-open')`)
	b.run(chromedp.KeyEvent(kb.Escape))
	b.check(`document.activeElement===document.getElementById('pv-open')`)
	// Create a different preview through the real API and htmx partial path.
	b.eval(`(async()=>{const response=await window.apiFetch('/api/artifacts',{method:'POST',body:JSON.stringify({title:'New preview',body:'<h1>New preview</h1>'})});const data=await response.json();artifact=data.artifact;window.nextArtifact=artifact;document.body.dispatchEvent(new CustomEvent('exhibit:artifact-saved'))})()`)
	b.wait(`window.nextArtifact && document.getElementById('pv-open').dataset.directOpenArtifact===nextArtifact.id`)
	b.eval(`window.tabs=[];window.open=()=>{const tab={opener:window,closed:false,location:null,close(){this.closed=true}};tabs.push(tab);return tab};document.getElementById('pv-open').click()`)
	b.run(chromedp.Click(chromedp.CSS(`#direct-open-confirm`)))
	b.wait(`tabs.length===1`)
	b.check(`tabs[0].location==='/artifacts/'+nextArtifact.id+'/open' && tabs[0].opener===null`)
}
