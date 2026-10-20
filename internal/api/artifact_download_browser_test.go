package api

import (
	"context"
	"fmt"
	"mime"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
	"github.com/momja/Exhibit/internal/chromium"
	"github.com/momja/Exhibit/internal/chromium/chromiumtest"
	"github.com/stretchr/testify/require"
)

// Exercise the real gallery/detail templates, shared credential client,
// Stimulus lifecycle, native dialog, htmx and blob download helper. The export
// response is a test boundary: its implementation belongs to the bundle API.
func TestArtifactDownloadBrowser(t *testing.T) {
	browser, err := chromium.New(chromiumtest.Start(t))
	require.NoError(t, err)
	ctx, cancel, err := browser.Tab(context.Background())
	require.NoError(t, err)
	defer cancel()
	ctx, cancel = context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	router := newTestRouter(t)
	seedGalleryArtifacts(t, router, 25)
	id := "gallery-024"
	var mode atomic.Int32
	type downloadRequest struct{ uri, auth string }
	requests := make(chan downloadRequest, 10)
	release := make(chan struct{}, 2)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/export") {
			requests <- downloadRequest{r.URL.RequestURI(), r.Header.Get("Authorization")}
			switch mode.Load() {
			case 0:
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprint(w, `{"error":"Saved asset unavailable. Try again."}`)
				return
			case 2:
				select {
				case <-release:
				case <-r.Context().Done():
					return
				}
			case 3:
				// A complete response header followed by an interrupted body.
				w.Header().Set("Content-Length", "1000")
				fmt.Fprint(w, "incomplete")
				return
			}
			format := r.URL.Query().Get("format")
			if format == "zip" {
				// The production ZIP writer emits an unquoted token filename.
				w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": "Portable-tool.zip"}))
			} else {
				w.Header().Set("Content-Disposition", `attachment; filename="Portable-tool.html"`)
			}
			fmt.Fprint(w, "portable-"+format)
			return
		}
		router.ServeHTTP(w, r)
	}))
	defer func() {
		close(release) // Also release an in-flight test response after a failure.
		srv.Close()
	}()
	router.cfg.AppOrigin = srv.URL

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
	request := func(expected string) {
		t.Helper()
		select {
		case got := <-requests:
			require.Equal(t, expected, got.uri)
			require.Equal(t, authHeader(), got.auth, "the shared client authenticates the export")
		case <-time.After(5 * time.Second):
			t.Fatal("no export request")
		}
	}
	const dialog = `[data-artifact-download-target="dialog"]`
	const submit = `[data-artifact-download-target="submit"]`
	const cancelButton = `[data-artifact-download-target="cancel"]`
	const zip = `[name="format"][value="zip"]`
	const html = `[name="format"][value="html"]`
	const observeDownloads = `window.downloads = []; window.downloadBlobs = new Map();
const createURL = URL.createObjectURL.bind(URL); URL.createObjectURL = blob => { const url = createURL(blob); downloadBlobs.set(url, blob); return url; };
const click = HTMLAnchorElement.prototype.click; HTMLAnchorElement.prototype.click = function() {
  if (this.download && this.href.startsWith('blob:')) {
    const name = this.download; downloadBlobs.get(this.href).text().then(body => downloads.push({name, body}));
  } else click.call(this);
};`
	openGallery := func(artifactID string) {
		t.Helper()
		run(chromedp.Click(chromedp.CSS(`#card-actions-`+artifactID+`-trigger`)),
			chromedp.KeyEvent(kb.End), chromedp.KeyEvent(kb.Enter),
			chromedp.Poll[chromedp.Void](`document.querySelector('`+dialog+`').open`))
	}
	run(chromedp.EmulateViewport(1280, 800), chromedp.Navigate(srv.URL),
		chromedp.Poll[chromedp.Void](`document.querySelector('[data-artifact-download-target="dialog"]') && document.querySelector('#card-actions-gallery-024-trigger')`),
		chromedp.Evaluate[chromedp.Void](observeDownloads))
	openGallery(id)
	check(`document.activeElement.matches('` + zip + `') && document.activeElement.checked`)
	check(`document.querySelector('[data-artifact-download-target="artifactTitle"]').textContent === 'Artifact 024'`)
	check(`document.querySelector('#card-actions-gallery-024').hidden`)
	// Native focus containment, radio keyboard choice, then Escape.
	run(chromedp.KeyEvent(kb.ArrowDown))
	check(`document.activeElement.matches('` + html + `') && document.activeElement.checked`)
	run(chromedp.KeyEvent(kb.Tab))
	check(`document.activeElement.matches('` + cancelButton + `')`)
	run(chromedp.KeyEvent(kb.Tab), chromedp.Evaluate[chromedp.Void](`document.querySelector('[data-gallery-query]').focus()`))
	check(`document.activeElement.matches('` + submit + `')`)
	run(chromedp.KeyEvent(kb.Escape), chromedp.Poll[chromedp.Void](`document.activeElement.id === 'card-actions-gallery-024-trigger'`))
	check(`!document.querySelector('` + dialog + `').open`)
	openGallery(id)
	check(`document.querySelector('` + zip + `').checked`)
	// The dialog's own padding does not count as its backdrop.
	run(chromedp.Evaluate[chromedp.Void](`{ const dialog = document.querySelector('` + dialog + `'); const box = dialog.getBoundingClientRect(); dialog.dispatchEvent(new MouseEvent('click', { bubbles:true, clientX:box.left+2, clientY:box.top+2 })); }`))
	check(`document.querySelector('` + dialog + `').open`)
	run(chromedp.MouseClickXY(1, 1), chromedp.Poll[chromedp.Void](`!document.querySelector('`+dialog+`').open`))
	check(`document.activeElement.id === 'card-actions-gallery-024-trigger'`)
	openGallery(id)
	run(chromedp.Click(chromedp.CSS(cancelButton)), chromedp.Poll[chromedp.Void](`document.activeElement.id === 'card-actions-gallery-024-trigger'`))
	require.Empty(t, requests, "opening or cancelling never downloads")

	// HTTP failure keeps the dialog and format choice available for retry.
	openGallery(id)
	run(chromedp.Click(chromedp.CSS(submit)), chromedp.Poll[chromedp.Void](`document.querySelector('`+submit+`').textContent === 'Try again'`))
	request("/api/artifacts/" + id + "/export?format=zip")
	check(`document.querySelector('` + dialog + `').open && document.activeElement.matches('` + submit + `') && !document.querySelector('` + submit + `').disabled`)
	check(`document.querySelector('[role="status"].artifact-download-status').textContent.includes('Saved asset unavailable') && downloads.length === 0`)
	mode.Store(1)
	run(chromedp.Click(chromedp.CSS(submit)), chromedp.Poll[chromedp.Void](`downloads.length === 1 && !document.querySelector('`+dialog+`').open`))
	request("/api/artifacts/" + id + "/export?format=zip")
	check(`downloads[0].name === 'Portable-tool.zip' && downloads[0].body === 'portable-zip'`)
	run(chromedp.Poll[chromedp.Void](`document.activeElement.id === 'card-actions-gallery-024-trigger'`))

	// One request at a time, including a repeated native submit event. All
	// dismissals use the same pending guard and cannot hide a failed request.
	openGallery(id)
	run(chromedp.Click(chromedp.CSS(html)))
	mode.Store(2)
	run(chromedp.Click(chromedp.CSS(submit)))
	request("/api/artifacts/" + id + "/export?format=html")
	check(`document.querySelector('` + submit + `').disabled && document.querySelector('` + cancelButton + `').disabled && document.querySelector('` + html + `').matches(':disabled')`)
	run(chromedp.KeyEvent(kb.Escape), chromedp.MouseClickXY(1, 1), chromedp.Evaluate[chromedp.Void](`document.querySelector('`+dialog+` form').dispatchEvent(new Event('submit', {bubbles:true,cancelable:true}))`))
	check(`document.querySelector('` + dialog + `').open`)
	require.Empty(t, requests)
	release <- struct{}{}
	run(chromedp.Poll[chromedp.Void](`downloads.length === 2 && !document.querySelector('` + dialog + `').open`))
	check(`downloads[1].name === 'Portable-tool.html' && downloads[1].body === 'portable-html'`)

	// A body-read/network failure is retryable too. Reopening resets errors
	// and selects the recommended ZIP again.
	openGallery(id)
	mode.Store(3)
	run(chromedp.Click(chromedp.CSS(submit)), chromedp.Poll[chromedp.Void](`document.querySelector('`+submit+`').textContent === 'Try again'`))
	request("/api/artifacts/" + id + "/export?format=zip")
	check(`downloads.length === 2 && document.querySelector('` + dialog + `').open`)
	run(chromedp.Click(chromedp.CSS(cancelButton)))
	openGallery(id)
	check(`document.querySelector('[data-artifact-download-target="status"]').textContent === '' && document.querySelector('` + zip + `').checked`)
	run(chromedp.Click(chromedp.CSS(cancelButton)))

	// Actual htmx append and replacement keep the stable dialog/controller.
	run(chromedp.Click(chromedp.CSS(`[data-gallery-page]`)), chromedp.Poll[chromedp.Void](`!!document.querySelector('#card-actions-gallery-000-trigger')`))
	openGallery("gallery-000")
	run(chromedp.Click(chromedp.CSS(cancelButton)),
		chromedp.SendKeys(chromedp.CSS(`[data-gallery-query]`), "Artifact 024"),
		chromedp.Poll[chromedp.Void](`document.querySelectorAll('.card').length === 1 && !document.querySelector('#card-actions-gallery-000-trigger')`))
	openGallery(id)
	run(chromedp.Click(chromedp.CSS(cancelButton)))
	check(`document.querySelectorAll('` + dialog + `').length === 1`)

	// Removing the stable host aborts its pending read and closes the native
	// dialog. Reconnecting the same host has no stale pending guard/listeners.
	// A queued close event from the preceding cancellation can arrive after
	// the action menu reopens; it must not steal the menu's keyboard focus.
	run(chromedp.Click(chromedp.CSS(`#card-actions-`+id+`-trigger`)),
		chromedp.Evaluate[chromedp.Void](`document.querySelector('`+dialog+`').dispatchEvent(new Event('close'))`),
		chromedp.KeyEvent(kb.End), chromedp.KeyEvent(kb.Enter),
		chromedp.Poll[chromedp.Void](`document.querySelector('`+dialog+`').open`))
	mode.Store(2)
	run(chromedp.Click(chromedp.CSS(submit)))
	request("/api/artifacts/" + id + "/export?format=zip")
	run(chromedp.Evaluate[chromedp.Void](`window.savedHost = document.querySelector('[data-gallery]'); savedHost.remove()`),
		chromedp.Poll[chromedp.Void](`!savedHost.querySelector('`+dialog+`').open`),
		chromedp.Evaluate[chromedp.Void](`document.body.prepend(savedHost)`))
	openGallery(id)
	mode.Store(1)
	run(chromedp.Click(chromedp.CSS(submit)), chromedp.Poll[chromedp.Void](`downloads.length === 3 && !document.querySelector('`+dialog+`').open`))
	request("/api/artifacts/" + id + "/export?format=zip")

	// The detail menu returns focus to its visible More trigger, rather than
	// the menu item pip.js hides. The same component fits a phone viewport.
	mode.Store(1)
	require.Equal(t, http.StatusOK, doJSON(t, router, "PATCH", "/api/artifacts/"+id, map[string]any{"title": `"Quoted"`}).Code)
	run(chromedp.EmulateViewport(390, 844), chromedp.Navigate(srv.URL+"/artifacts/"+id),
		chromedp.Evaluate[chromedp.Void](observeDownloads), chromedp.Click(chromedp.CSS(`#pip-more`)),
		chromedp.Click(chromedp.CSS(`[data-action="artifact-download#open"]`)),
		chromedp.Poll[chromedp.Void](`document.querySelector('`+dialog+`').open`))
	check(`document.querySelector('#pip-menu').hidden && document.activeElement.matches('` + zip + `')`)
	check(`document.querySelector('[data-artifact-download-target="artifactTitle"]').textContent === '"Quoted"'`)
	check(`{ const box = document.querySelector('` + dialog + `').getBoundingClientRect(); box.left >= 0 && box.right <= innerWidth && box.top >= 0 && box.bottom <= innerHeight; }`)
	run(chromedp.KeyEvent(kb.Escape), chromedp.Poll[chromedp.Void](`document.activeElement.id === 'pip-more'`),
		chromedp.Click(chromedp.CSS(`#pip-more`)), chromedp.Click(chromedp.CSS(`[data-action="artifact-download#open"]`)),
		chromedp.Click(chromedp.CSS(html)), chromedp.Click(chromedp.CSS(submit)),
		chromedp.Poll[chromedp.Void](`downloads.length === 1 && !document.querySelector('`+dialog+`').open`))
	request("/api/artifacts/" + id + "/export?format=html")
	check(`downloads[0].body === 'portable-html'`)
	run(chromedp.Poll[chromedp.Void](`document.activeElement.id === 'pip-more'`))
}
