package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/chromedp"
	"github.com/momja/Exhibit/internal/chromium"
	"github.com/momja/Exhibit/internal/chromium/chromiumtest"
	"github.com/momja/Exhibit/internal/connectors"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/require"
)

// Exh-sez0: the destination warning is a step in the trusted connect form, not
// an error after the fact. This drives the real page: type an Anthropic key at
// another host, see the warning name both destinations, confirm, and watch the
// connection land in the list. Both destinations are checked at desktop and
// phone widths because this is the screen where a person decides.
func TestConnectionDestinationWarningBrowser(t *testing.T) {
	in := newProfileInstance(t, "origin-warning", "origin@example.test")
	in.ro.cfg.Relay.Enabled = true
	in.ro.cfg.Connections = connectors.New(connectors.Config{CustomEnabled: true, Store: in.ro.cfg.Store.(*store.SQLiteStore), Box: in.ro.cfg.Secrets})
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.AddCookie(in.cookie)
		in.ro.ServeHTTP(w, r)
	}))
	defer app.Close()
	in.ro.cfg.AppOrigin = app.URL

	browser, err := chromium.New(chromiumtest.Start(t))
	require.NoError(t, err)
	ctx, cancel, err := browser.Tab(context.Background())
	require.NoError(t, err)
	defer cancel()
	ctx, cancel = context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	run := func(t *testing.T, actions ...chromedp.Action[chromedp.Void]) {
		t.Helper()
		require.NoError(t, chromedp.Do(ctx, actions...))
	}
	check := func(t *testing.T, expression string) {
		t.Helper()
		ok, err := chromedp.Run(ctx, chromedp.Evaluate[bool](expression))
		require.NoError(t, err)
		require.True(t, ok, expression)
	}
	// A full navigation destroys the execution context under an in-flight
	// evaluation, which is not a failure of the page: re-evaluate until the new
	// document answers.
	waitFor := func(t *testing.T, expression string, budget time.Duration) {
		t.Helper()
		deadline := time.Now().Add(budget)
		for {
			ok, err := chromedp.Run(ctx, chromedp.Evaluate[bool](expression))
			if err == nil && ok {
				return
			}
			if time.Now().After(deadline) {
				require.NoError(t, err, expression)
				require.True(t, ok, expression)
				return
			}
			time.Sleep(150 * time.Millisecond)
		}
	}
	capture := func(t *testing.T, name string) {
		t.Helper()
		dir := os.Getenv("CONNECTION_ORIGIN_SCREENSHOT_DIR")
		if dir == "" {
			return
		}
		require.NoError(t, os.MkdirAll(dir, 0755))
		// A navigation in flight invalidates the target mid-capture; the shot
		// is evidence, so retry rather than fail the behaviour test on it.
		var shot []byte
		var err error
		for attempt := 0; attempt < 5; attempt++ {
			shot, err = chromedp.Run(ctx, chromedp.Screenshot(chromedp.CSS("body")))
			if err == nil {
				require.NoError(t, os.WriteFile(filepath.Join(dir, name), shot, 0644))
				return
			}
			_, _ = chromedp.Run(ctx, chromedp.Sleep(300*time.Millisecond))
		}
		require.NoError(t, err)
	}

	for _, width := range []int64{1280, 390} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			_, err := chromedp.Call(ctx, emulation.SetDeviceMetricsOverride, emulation.SetDeviceMetricsOverrideParams{Width: width, Height: 900, DeviceScaleFactor: 1})
			require.NoError(t, err)
			run(t, chromedp.Navigate(app.URL+"/connections?advanced=1"),
				chromedp.Poll[chromedp.Void](`document.querySelector('[data-connections-target="createForm"]')`))

			// A Basic credential URL stays in the DOM when the person switches
			// to bearer, but it is disabled and is not the submitted destination.
			run(t, chromedp.Evaluate[chromedp.Void](`{const kind=document.getElementById('connection-kind');kind.value='basic';kind.dispatchEvent(new Event('change',{bubbles:true}))}`))
			run(t, chromedp.SendKeys(chromedp.CSS(`#connection-access-url`), "https://basic-user:basic-pass@basic.example.test/base"))
			check(t, `document.querySelector('[data-connections-target="destinationHost"]').textContent === 'basic.example.test'`)
			run(t, chromedp.Evaluate[chromedp.Void](`{const kind=document.getElementById('connection-kind');kind.value='bearer';kind.dispatchEvent(new Event('change',{bubbles:true}))}`))
			check(t, `(() => {
   const destination = document.querySelector('[data-connections-target="destination"]');
   const access = document.getElementById('connection-access-url');
   return access.disabled && destination.hidden;
  })()`)
			run(t, chromedp.SendKeys(chromedp.CSS(`#connection-origin`), "https://api-openai.co"))
			// The destination is named before anything is pasted, from the
			// recipe that will actually submit.
			check(t, `document.querySelector('[data-connections-target="destinationHost"]').textContent === 'api-openai.co' && !document.querySelector('[data-connections-target="destination"]').hidden`)
			run(t, chromedp.SendKeys(chromedp.CSS(`#connection-name`), "Anthropic account"))
			run(t, chromedp.SendKeys(chromedp.CSS(`#connection-secret`), "sk-ant-api03-not-a-real-key"))
			run(t, chromedp.Click(chromedp.CSS(`.connect-submit`)))

			waitFor(t, `!document.querySelector('[data-connections-target="createWarning"]').hidden`, 10*time.Second)
			check(t, `(() => {
   const text = document.querySelector('[data-connections-target="createWarningText"]').textContent;
   const button = document.querySelector('[data-connections-target="confirmButton"]').textContent;
   return text.includes('Anthropic API key') && text.includes('api.anthropic.com') && text.includes('api-openai.co') && button.includes('api-openai.co');
  })()`)
			// It is a decision, not a dead end: the form is still there to edit.
			check(t, `!!document.getElementById('connection-origin')`)
			capture(t, fmt.Sprintf("warning-%d.png", width))

			// Editing after the warning drops it. A confirmation describes the
			// values that were submitted, so it cannot survive a change.
			run(t, chromedp.Evaluate[chromedp.Void](`{const origin=document.getElementById('connection-origin');origin.value='https://api-anthropic.example';origin.dispatchEvent(new Event('input',{bubbles:true}))}`))
			waitFor(t, `document.querySelector('[data-connections-target="createWarning"]').hidden`, 5*time.Second)
			// A recipe change is the form's change event and invalidates too.
			run(t, chromedp.Evaluate[chromedp.Void](`{const kind=document.getElementById('connection-kind');kind.value='basic';kind.dispatchEvent(new Event('change',{bubbles:true}))}`))
			waitFor(t, `document.querySelector('[data-connections-target="createWarning"]').hidden`, 5*time.Second)
			run(t, chromedp.Evaluate[chromedp.Void](`{const kind=document.getElementById('connection-kind');kind.value='bearer';kind.dispatchEvent(new Event('change',{bubbles:true}))}`))
			waitFor(t, `document.querySelector('[data-connections-target="createWarning"]').hidden`, 5*time.Second)
			// Switching back restores the fields bearer submits, and leaves the
			// Basic credential URL disabled rather than sending it.
			check(t, `(() => {
   const form = document.querySelector('[data-connections-target="createForm"]');
   return form.elements.recipe.value === 'bearer' && !form.elements.origin.disabled &&
     form.elements.origin.value === 'https://api-anthropic.example' &&
     !form.elements.secret.disabled && form.elements.secret.value.length > 0 &&
     form.elements.access_url.disabled;
  })()`)
			// Only a fresh normal submission may warn again, and it warns about
			// the destination now on screen.
			run(t, chromedp.Click(chromedp.CSS(`.connect-submit`)))
			waitFor(t, `!document.querySelector('[data-connections-target="createWarning"]').hidden`, 10*time.Second)
			check(t, `(() => {
   const text = document.querySelector('[data-connections-target="createWarningText"]').textContent;
   const button = document.querySelector('[data-connections-target="confirmButton"]').textContent;
   return text.includes('api.anthropic.com') && text.includes('api-anthropic.example') && button.includes('api-anthropic.example');
  })()`)
			capture(t, fmt.Sprintf("warning-edited-%d.png", width))

			run(t, chromedp.Click(chromedp.CSS(`[data-connections-target="confirmButton"]`)))
			waitFor(t, `location.search.includes('connected=1') && document.readyState === 'complete' && document.body.textContent.includes('Anthropic account')`, 15*time.Second)
			check(t, `document.body.textContent.includes('Anthropic account')`)
			capture(t, fmt.Sprintf("saved-%d.png", width))
		})
	}
}
