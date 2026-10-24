package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
	"github.com/momja/Exhibit/internal/chromium"
	"github.com/momja/Exhibit/internal/chromium/chromiumtest"
	"github.com/stretchr/testify/require"
)

// The small Node DOM cannot exercise MutationObserver, real focus, or native
// event propagation. Run the actual partial, vendored Stimulus, and htmx in the
// same Chromium test infrastructure already used by other browser consumers.
func TestActionMenuBrowser(t *testing.T) {
	browser, err := chromium.New(chromiumtest.Start(t))
	require.NoError(t, err)
	ctx, cancel, err := browser.Tab(context.Background())
	require.NoError(t, err)
	defer cancel()
	ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	artifact := actionMenuView{ID: "artifact-actions", Label: "Artifact actions", Items: []actionMenuItem{
		menuLink("Edit", "ph-pencil-simple", "#edit"),
		menuEvent("Duplicate", "ph-copy", "example:duplicate", map[string]string{"artifactID": "alpha", "title": `Alpha "<& title`}),
	}}
	collection := actionMenuView{ID: "collection-actions", Label: "Collection actions", Items: []actionMenuItem{
		menuEvent("Rename", "ph-pencil-simple", "example:rename", map[string]string{"collectionID": "research"}),
		menuEvent("Sort by title", "ph-sort-ascending", "example:sort", map[string]string{"collectionID": "research"}),
	}}
	menus := func() string {
		first, err := renderPage("actionMenu", artifact)
		require.NoError(t, err)
		second, err := renderPage("actionMenu", collection)
		require.NoError(t, err)
		return first + second
	}
	mux := http.NewServeMux()
	mux.Handle("/assets/", assetsHandler())
	mux.HandleFunc("GET /menus", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, menus())
	})
	mux.HandleFunc("GET /append", func(w http.ResponseWriter, r *http.Request) {
		third := collection
		third.ID = "appended-actions"
		fragment, err := renderPage("actionMenu", third)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, fragment)
	})
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!doctype html><html lang="en"><head><meta charset="utf-8">
<link rel="stylesheet" href="/assets/gallery/tokens.css"><link rel="stylesheet" href="/assets/gallery/components.css">
<script src="/assets/htmx/htmx.min.js"></script></head><body>
<button id="before">Before</button><div id="menus" style="padding:80px">`, menus(), `</div>
<button id="replace" hx-get="/menus" hx-target="#menus" hx-swap="innerHTML">Replace</button>
<button id="append" hx-get="/append" hx-target="#menus" hx-swap="beforeend">Append</button>
<button id="after">After</button><div id="edit">Edit destination</div>
<script type="module" src="/assets/gallery/controllers.js"></script>
<script type="module">
import { application } from '/assets/gallery/controllers.js';
window.application = application;
window.selections = [];
for (const name of ['example:duplicate', 'example:rename', 'example:sort']) {
  document.addEventListener(name, event => {
    window.selections.push({ event: event.type, ...event.detail, trigger: event.detail.trigger.id });
  });
}
</script></body></html>`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

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
	run(chromedp.Navigate(srv.URL), chromedp.Poll[chromedp.Void](`window.application?.controllers.length === 2`))

	// Both instances share a controller while exposing different actions.
	run(chromedp.Click(chromedp.CSS(`#artifact-actions-trigger`)))
	check(`document.activeElement.textContent.trim() === 'Edit'`)
	run(chromedp.KeyEvent(kb.ArrowDown))
	check(`document.activeElement.textContent.trim() === 'Duplicate'`)
	run(chromedp.KeyEvent(kb.Enter))
	check(`selections.length === 1 && selections[0].event === 'example:duplicate' && selections[0].artifactID === 'alpha' && selections[0].title === 'Alpha "<& title' && selections[0].trigger === 'artifact-actions-trigger'`)
	check(`document.querySelector('#artifact-actions').hidden && document.activeElement.id === 'artifact-actions-trigger'`)
	run(chromedp.Click(chromedp.CSS(`#collection-actions-trigger`)), chromedp.KeyEvent(kb.End), chromedp.KeyEvent(kb.Enter))
	check(`selections.length === 2 && selections[1].event === 'example:sort' && selections[1].collectionID === 'research'`)

	// Keyboard entry, wrapping, typeahead, Escape, and ordinary Tab exit.
	run(chromedp.Focus(chromedp.CSS(`#artifact-actions-trigger`)), chromedp.KeyEvent(kb.ArrowUp))
	check(`document.activeElement.textContent.trim() === 'Duplicate'`)
	run(chromedp.KeyEvent(kb.ArrowDown))
	check(`document.activeElement.textContent.trim() === 'Edit'`)
	run(chromedp.KeyEvent("d"))
	check(`document.activeElement.textContent.trim() === 'Duplicate'`)
	run(chromedp.KeyEvent(kb.Escape))
	check(`document.querySelector('#artifact-actions').hidden && document.activeElement.id === 'artifact-actions-trigger'`)
	run(chromedp.KeyEvent(kb.ArrowDown), chromedp.KeyEvent(kb.Tab))
	check(`document.querySelector('#artifact-actions').hidden && document.activeElement.id === 'collection-actions-trigger'`)

	// Opening another instance closes the first. A null Safari focusout keeps
	// its pending click available, while clicking outside closes the menu.
	run(chromedp.Click(chromedp.CSS(`#artifact-actions-trigger`)), chromedp.Click(chromedp.CSS(`#collection-actions-trigger`)))
	check(`document.querySelector('#artifact-actions').hidden && !document.querySelector('#collection-actions').hidden`)
	run(chromedp.Evaluate[chromedp.Void](`document.activeElement.dispatchEvent(new FocusEvent('focusout', { bubbles: true, relatedTarget: null }))`))
	check(`!document.querySelector('#collection-actions').hidden`)
	run(chromedp.Click(chromedp.CSS(`#after`)))
	check(`document.querySelector('#collection-actions').hidden && document.activeElement.id === 'after'`)

	// Real htmx insertion/replacement: no manual initialization or swap hook.
	run(chromedp.Evaluate[chromedp.Void](`window.oldMenu = document.querySelector('[data-controller="action-menu"]')`),
		chromedp.Click(chromedp.CSS(`#replace`)),
		chromedp.Poll[chromedp.Void](`!oldMenu.isConnected && application.controllers.length === 2`))
	check(`application.getControllerForElementAndIdentifier(oldMenu, 'action-menu') === null`)
	run(chromedp.Click(chromedp.CSS(`#artifact-actions-trigger`)), chromedp.KeyEvent(kb.End), chromedp.KeyEvent(kb.Enter))
	check(`selections.length === 3 && selections[2].event === 'example:duplicate'`)
	run(chromedp.Click(chromedp.CSS(`#append`)), chromedp.Poll[chromedp.Void](`application.controllers.length === 3`),
		chromedp.Click(chromedp.CSS(`#appended-actions-trigger`)), chromedp.KeyEvent(kb.Enter))
	check(`selections.length === 4 && selections[3].event === 'example:rename'`)

	// Disconnect removes global actions; reconnecting the same node produces
	// one functioning instance, with its transient menu state reset.
	run(chromedp.Click(chromedp.CSS(`#appended-actions-trigger`)),
		chromedp.Evaluate[chromedp.Void](`window.savedMenu = document.querySelector('#appended-actions-trigger').parentElement; savedMenu.remove()`),
		chromedp.Poll[chromedp.Void](`application.controllers.length === 2`))
	check(`savedMenu.querySelector('[role="menu"]').hidden && !savedMenu.classList.contains('is-open')`)
	run(chromedp.Evaluate[chromedp.Void](`document.querySelector('#menus').append(savedMenu)`), chromedp.Poll[chromedp.Void](`application.controllers.length === 3`),
		chromedp.Click(chromedp.CSS(`#appended-actions-trigger`)), chromedp.KeyEvent(kb.End), chromedp.KeyEvent(kb.Enter))
	check(`selections.length === 5 && selections[4].event === 'example:sort'`)
}

func TestGalleryActionMenuDuplicationBrowser(t *testing.T) {
	browser, err := chromium.New(chromiumtest.Start(t))
	require.NoError(t, err)
	ctx, cancel, err := browser.Tab(context.Background())
	require.NoError(t, err)
	defer cancel()
	ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	router := newTestRouter(t)
	id := createTestArtifact(t, router, "Original")
	srv := httptest.NewServer(router)
	defer srv.Close()
	router.cfg.AppOrigin = srv.URL
	trigger := `#card-actions-` + id + `-trigger`
	menu := `#card-actions-` + id
	require.NoError(t, chromedp.Do(ctx,
		chromedp.Navigate(srv.URL),
		chromedp.Focus(chromedp.CSS(trigger)),
		chromedp.Poll[chromedp.Void](`document.querySelector('`+trigger+`').getAttribute('aria-expanded') === 'false'`),
		chromedp.KeyEvent(kb.ArrowDown), chromedp.KeyEvent(kb.End), chromedp.KeyEvent(kb.Enter),
		chromedp.Poll[chromedp.Void](`document.querySelector('[data-duplicate-dialog]').open`),
	))
	correct, err := chromedp.Run(ctx, chromedp.Evaluate[bool](`document.querySelector('`+menu+`').hidden && document.querySelector('[data-duplicate-dialog] [name="title"]').value === 'Original (copy)'`))
	require.NoError(t, err)
	require.True(t, correct)
	require.NoError(t, chromedp.Do(ctx, chromedp.KeyEvent(kb.Escape)))
	correct, err = chromedp.Run(ctx, chromedp.Evaluate[bool](`document.activeElement.id === '`+trigger[1:]+`'`))
	require.NoError(t, err)
	require.True(t, correct, "closing the feature dialog restores its visible menu trigger")

	// Removing an open modal releases its native top-layer state. Reconnecting
	// the same element produces one functioning controller, with fresh focus.
	require.NoError(t, chromedp.Do(ctx,
		chromedp.KeyEvent(kb.ArrowDown), chromedp.KeyEvent(kb.End), chromedp.KeyEvent(kb.Enter),
		chromedp.Poll[chromedp.Void](`document.querySelector('[data-duplicate-dialog]').open`),
		chromedp.Evaluate[chromedp.Void](`window.removedModal = document.querySelector('[data-duplicate-dialog]'); window.modalParent = removedModal.parentElement; removedModal.remove()`),
		chromedp.Poll[chromedp.Void](`!removedModal.open`),
		chromedp.Evaluate[chromedp.Void](`modalParent.append(removedModal)`),
		chromedp.Focus(chromedp.CSS(trigger)),
		chromedp.KeyEvent(kb.ArrowDown), chromedp.KeyEvent(kb.End), chromedp.KeyEvent(kb.Enter),
		chromedp.Poll[chromedp.Void](`removedModal.open && document.activeElement === removedModal.querySelector('[name="title"]')`),
		chromedp.KeyEvent(kb.Escape),
	))
	correct, err = chromedp.Run(ctx, chromedp.Evaluate[bool](`!removedModal.open && document.activeElement.id === '`+trigger[1:]+`'`))
	require.NoError(t, err)
	require.True(t, correct, "reconnected modal dismisses and restores focus")
}
