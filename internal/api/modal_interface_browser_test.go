package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
)

// Exercise the same handle features import, with actual native dialogs and the
// registered Stimulus lifecycle. No application/controller internals are used.
func TestModalInterfaceBrowser(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("/assets/", assetsHandler())
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!doctype html><html><head>
<link rel="stylesheet" href="/assets/gallery/tokens.css">
<link rel="stylesheet" href="/assets/gallery/components.css"></head><body>
<button id="trigger">Open</button><div id="dialogs">
<dialog data-controller="modal" class="modal-dialog" aria-label="First">
<input autofocus aria-label="Working copy" value="unsaved">
<button type="button" data-modal-dismiss>Cancel</button>
<button type="button" disabled>Unavailable</button><button type="button">Save</button></dialog>
<dialog data-controller="modal" class="modal-dialog" aria-label="Second">
<input autofocus aria-label="Other working copy"><button type="button" data-modal-dismiss>Done</button></dialog>
</div><script type="module">
import { modalFor } from '/assets/gallery/modal.js';
window.dialogs = [...document.querySelectorAll('dialog')];
window.first = modalFor(dialogs[0]); window.second = modalFor(dialogs[1]);
window.trigger = document.querySelector('#trigger');
// Opening works even before the controller module starts.
window.earlyOpened = first.show(trigger) && dialogs[0].open;
first.close();
await import('/assets/gallery/controllers.js');
window.ready = true;
</script></body></html>`)
	})
	app := httptest.NewServer(mux)
	defer app.Close()
	b := newModalBrowser(t, 1280)
	b.run(chromedp.Navigate(app.URL))
	b.wait(`window.ready`)
	b.check(`earlyOpened && !dialogs[0].open`)
	b.eval(`first.show(trigger)`)
	b.check(`document.activeElement === dialogs[0].querySelector('input')`)
	b.eval(`dialogs[0].querySelector('input').focus()`)
	b.run(chromedp.KeyEvent(kb.Tab, chromedp.KeyModifiers(chromedp.ModifierShift)))
	b.check(`document.activeElement === dialogs[0].querySelector('button:last-child')`)
	b.run(chromedp.KeyEvent(kb.Tab))
	b.check(`document.activeElement === dialogs[0].querySelector('input')`)
	// Padding is content; only coordinates outside the dialog dismiss it.
	b.eval(`{ const d=dialogs[0], r=d.getBoundingClientRect(); d.dispatchEvent(new MouseEvent('click', {bubbles:true,clientX:r.left+2,clientY:r.top+2})); }`)
	b.check(`dialogs[0].open`)
	b.eval(`window.result = first.run(() => new Promise(resolve => window.finish = resolve)); window.duplicateCalls=0; first.run(() => duplicateCalls++)`)
	b.check(`first.pending && duplicateCalls===0 && [...dialogs[0].querySelectorAll('button')].every(b=>b.disabled)`)
	b.run(chromedp.KeyEvent(kb.Escape))
	b.eval(`dialogs[0].dispatchEvent(new MouseEvent('click', {bubbles:true,clientX:-1,clientY:-1})); dialogs[0].querySelector('[data-modal-dismiss]').click(); first.close()`)
	b.check(`dialogs[0].open`)
	b.eval(`finish('saved')`)
	b.wait(`!first.pending`)
	b.check(`!dialogs[0].querySelector('button:last-child').disabled && dialogs[0].querySelector('button[disabled]').textContent==='Unavailable' && dialogs[0].querySelector('input').value==='unsaved'`)
	b.eval(`first.run(() => { throw Error('feature error'); }).catch(error => window.failure = error.message)`)
	b.wait(`window.failure==='feature error' && !first.pending`)
	b.eval(`dialogs[0].querySelector('[data-modal-dismiss]').click()`)
	b.check(`!dialogs[0].open && document.activeElement===trigger`)
	// A queued native close cannot restore focus over the reopened working copy.
	b.eval(`first.show(trigger); first.close(); first.show(trigger)`)
	b.wait(`dialogs[0].open && document.activeElement===dialogs[0].querySelector('input')`)
	// Disconnect cancels the old handle operation's completion, restores controls,
	// and reconnects the same node without listeners or pending state accumulating.
	b.eval(`window.old = first.run(() => new Promise(resolve => window.finishOld = resolve)); dialogs[0].remove()`)
	b.wait(`!dialogs[0].open && !first.pending`)
	b.eval(`document.querySelector('#dialogs').append(dialogs[0]); first.show(trigger)`)
	b.wait(`dialogs[0].open`)
	b.eval(`window.current = first.run(() => new Promise(resolve => window.finishCurrent = resolve)); finishOld('stale'); old.then(value => window.oldIgnored = value===undefined)`)
	b.wait(`window.oldIgnored`)
	b.check(`first.pending && dialogs[0].querySelector('button:last-child').disabled`)
	b.eval(`finishCurrent('new')`)
	b.wait(`!first.pending`)
	b.run(chromedp.KeyEvent(kb.Escape))
	b.check(`!dialogs[0].open && document.activeElement===trigger`)
	// Separate instances use their own focus fields and pending state.
	b.eval(`second.show(trigger); window.other = second.run(() => new Promise(resolve => window.finishOther = resolve))`)
	b.check(`dialogs[1].open && !first.pending && second.pending && document.activeElement===dialogs[1].querySelector('input')`)
	b.eval(`finishOther(); other.then(() => second.close())`)
	b.wait(`!dialogs[1].open && document.activeElement===trigger`)
}
