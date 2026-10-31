package api

import (
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
	"github.com/momja/Exhibit/internal/chromium"
	"github.com/momja/Exhibit/internal/chromium/chromiumtest"
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
			b.capture("Exh-9xfp", width)
			b.eval(`document.getElementById('share-add-input').value='unfinished';document.getElementById('share-add-input').dispatchEvent(new Event('input',{bubbles:true})); document.getElementById('state-how-it-works').open=true;document.getElementById('share-state-shared').click()`)
			b.wait(`document.getElementById('share-state-mode').dataset.savedMode==='shared' && !document.getElementById('share-close').disabled`)
			b.check(`document.getElementById('share-panel').open && document.getElementById('share-add-input').value==='unfinished' && document.getElementById('state-how-it-works').open`)
			b.eval(`document.getElementById('share-close').click()`)
			b.wait(`!document.getElementById('share-panel').open`)
			// Short-window content, including Done, remains reachable by scrolling.
			_, err := chromedp.Call(b.ctx, emulation.SetDeviceMetricsOverride, emulation.SetDeviceMetricsOverrideParams{Width: width, Height: 400, DeviceScaleFactor: 1})
			require.NoError(t, err)
			b.eval(open)
			b.eval(`document.getElementById('share-close').scrollIntoView()`)
			b.check(`document.getElementById('share-close').getBoundingClientRect().bottom<=400 && document.getElementById('share-add-input').value===''`)
		})
	}
}
