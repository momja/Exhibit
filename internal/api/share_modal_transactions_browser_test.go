package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
)

func TestShareModalTransactionsBrowser(t *testing.T) {
	ro := newTestRouter(t)
	seedLocalAccount(t, ro, "alice")
	id := createTestArtifact(t, ro, "Sharing transactions")
	// Plans control real fragment responses: a delayed swap, server error, and
	// lost response. The browser still uses actual htmx and authenticated APIs.
	plans := make(chan chan int, 3)
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/partials/share-panel" {
			select {
			case plan := <-plans:
				select {
				case status := <-plan:
					if status >= 500 {
						http.Error(w, "temporary refresh failure", status)
						return
					}
				case <-r.Context().Done():
					return
				}
			default:
			}
		}
		ro.ServeHTTP(w, r)
	}))
	defer app.Close()
	ro.cfg.AppOrigin = app.URL
	b := newModalBrowser(t, 1280)
	b.run(chromedp.Navigate(app.URL + "/artifacts/" + id))
	b.wait(`document.getElementById('share-open') && window.apiFetch`)
	b.eval(`document.getElementById('share-open').click();window.shareCalls=[];window.shareAPI=window.apiFetch;
window.apiFetch=(path,options)=>{shareCalls.push(options.method+' '+path);return shareAPI(path,options)};
window.shareInput=document.getElementById('share-add-input');
shareInput.value='alice, typo';shareInput.dispatchEvent(new Event('input',{bubbles:true}));
shareInput.dispatchEvent(new CompositionEvent('compositionstart',{bubbles:true}));
document.getElementById('share-add-form').requestSubmit()`)
	b.check(`shareCalls.length===0`)
	delayed := make(chan int, 1)
	plans <- delayed
	b.eval(`shareInput.dispatchEvent(new CompositionEvent('compositionend',{bubbles:true}));document.getElementById('state-how-it-works').open=true;document.getElementById('share-add-form').requestSubmit()`)
	b.wait(`document.getElementById('share-status').textContent.includes('Check these names') && document.getElementById('share-close').disabled`)
	b.check(`document.getElementById('share-add-input').matches(':disabled') && !document.getElementById('share-grants').textContent.includes('alice')`)
	b.run(chromedp.KeyEvent(kb.Escape))
	b.eval(`document.getElementById('share-panel').dispatchEvent(new MouseEvent('click',{bubbles:true,clientX:-1,clientY:-1}));
document.getElementById('share-close').click();document.getElementById('share-add-form').dispatchEvent(new Event('submit',{bubbles:true,cancelable:true}));document.getElementById('share-link-toggle').click()`)
	b.check(`document.getElementById('share-panel').open && shareCalls.length===1`)
	delayed <- 200
	b.wait(`!document.getElementById('share-close').disabled && document.getElementById('share-grants').textContent.includes('alice')`)
	b.check(`document.getElementById('share-add-input').value==='typo' && document.activeElement.id==='share-add-input' && document.getElementById('state-how-it-works').open`)
	// A committed link mutation with a failed GET must never reuse the old ID
	// or replay the mutation when the person closes and reopens the dialog.
	failed := make(chan int, 1)
	plans <- failed
	b.eval(`document.getElementById('share-link-toggle').click()`)
	b.wait(`document.getElementById('share-close').disabled && document.getElementById('share-status').textContent.includes('Changes saved')`)
	failed <- 500
	b.wait(`!document.getElementById('share-close').disabled && document.getElementById('share-status').textContent.includes('could not refresh')`)
	b.check(`document.getElementById('share-body').disabled && document.getElementById('share-add-input').value==='typo'`)
	b.eval(`document.getElementById('share-link-toggle').checked=false;document.getElementById('share-link-toggle').dispatchEvent(new Event('change',{bubbles:true}))`)
	b.check(`shareCalls.length===2`)
	b.eval(`document.getElementById('share-close').click();document.getElementById('share-open').click()`)
	b.wait(`!document.getElementById('share-body').disabled && !!document.getElementById('share-link-toggle').dataset.shareId`)
	b.check(`shareCalls.length===2 && document.getElementById('share-add-input').value===''`)
	b.eval(`window.linkID=document.getElementById('share-link-toggle').dataset.shareId;document.getElementById('share-link-toggle').click()`)
	b.wait(`!document.getElementById('share-close').disabled && !document.getElementById('share-link-toggle').dataset.shareId`)
	b.check(`shareCalls.length===3 && shareCalls[2]==='DELETE /api/shares/'+linkID`)
	// htmx already bounds lost GETs. Exercise its real AbortSignal/error path
	// with a short test timeout rather than adding another modal timer.
	plans <- make(chan int)
	b.eval(`htmx.config.defaultTimeout=100;document.getElementById('share-state-shared').click()`)
	b.wait(`!document.getElementById('share-close').disabled && document.getElementById('share-status').textContent.includes('could not refresh')`)
	b.check(`document.getElementById('share-body').disabled && shareCalls.length===4`)
	b.eval(`htmx.config.defaultTimeout=60000;document.getElementById('share-close').click();document.getElementById('share-open').click()`)
	b.wait(`!document.getElementById('share-body').disabled && document.getElementById('share-state-mode').dataset.savedMode==='shared'`)
	b.check(`shareCalls.length===4`)
	// Reconnect an idle feature root: the same handle opens and dismisses once.
	b.eval(`window.savedPanel=document.getElementById('share-panel');window.savedParent=savedPanel.parentElement;savedPanel.remove()`)
	b.wait(`!savedPanel.open`)
	b.eval(`savedParent.append(savedPanel);document.getElementById('share-open').click()`)
	b.wait(`savedPanel.open`)
	b.run(chromedp.KeyEvent(kb.Escape))
	b.check(`!savedPanel.open && document.activeElement.id==='share-open'`)
}
