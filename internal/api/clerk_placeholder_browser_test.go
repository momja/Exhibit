package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
	"github.com/momja/Exhibit/internal/chromium"
	"github.com/momja/Exhibit/internal/chromium/chromiumtest"
	"github.com/stretchr/testify/require"
)

// Only the remote SDK is doubled. Production templates, CSS, deferred loader,
// permission reload, Stimulus and htmx all execute in real Chromium.
func clerkPlaceholderApp(t *testing.T) *httptest.Server {
	t.Helper()
	ro, st := newLoginTestRouter(t, nil, nil, func(c *Config) {
		c.Clerk = hostedLoginStub{}
		c.ClerkPublishableKey = "pk_test_placeholder"
		c.AuthToken = ""
	})
	user, err := st.UpsertHostedUser(context.Background(), "clerk:placeholder", "person@example.test", time.Now().UnixMilli())
	require.NoError(t, err)
	seedGalleryFilters(t, ro, user.ID)
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/npm/") {
			w.Header().Set("Content-Type", "text/javascript")
			if strings.Contains(r.URL.Path, "clerk-js") {
				fmt.Fprint(w, clerkPlaceholderSDK)
			} else {
				fmt.Fprint(w, `window.__internal_ClerkUICtor = function() {};`)
			}
			return
		}
		r.Header.Set("Test-Clerk-Subject", "clerk:placeholder")
		ro.ServeHTTP(w, r)
	}))
	t.Cleanup(app.Close)
	ro.cfg.AppOrigin, ro.cfg.ClerkFrontendAPI = app.URL, app.URL
	return app
}

const clerkPlaceholderSDK = `
const mode = new URLSearchParams(location.search).get('account-test');
window.clerkCalls = {load:0,reload:0,mount:0,unmount:0};
window.reloads = [];
let renderTimer;
window.Clerk = {
 isSignedIn: mode !== 'signed-out',
 async load() {
  clerkCalls.load++;
  if (mode === 'load-failure') throw new Error('Account service unavailable. Please reload to try again.');
 },
 user: {reload() {
  clerkCalls.reload++;
  if (mode === 'reload-failure') return Promise.reject(new Error('Account permissions could not be refreshed.'));
  return new Promise(resolve => { window.reloads.push(resolve); window.finishReload = resolve; if (mode === 'ready') resolve(); });
 }},
 mountUserButton(root, options) {
  clerkCalls.mount++;
  const wrapper = document.createElement('div'); root.append(wrapper);
  window.renderButton = () => {
   const button = document.createElement('button');
   button.className = 'cl-userButtonTrigger btn btn-sec';
   button.setAttribute('aria-label', 'Your account'); button.disabled = true;
   Object.assign(button.style, options.appearance.elements.userButtonTrigger);
   const glyph = document.createElement('i'); glyph.className = 'ph ph-user-circle'; glyph.setAttribute('aria-hidden', 'true'); button.append(glyph);
   button.onclick = () => { window.accountClicks = (window.accountClicks || 0) + 1; };
   wrapper.append(button);
   window.enableButton = () => { button.disabled = false; };
  };
  if (mode === 'ready') renderTimer = setTimeout(() => { renderButton(); enableButton(); }, 100);
 },
 unmountUserButton(root) { clerkCalls.unmount++; clearTimeout(renderTimer); root.replaceChildren(); },
};`

// Hold the actual loader at its idle boundary, then release it explicitly.
// The production cap and near-expiry session behavior have Node coverage.
const clerkPlaceholderHoldIdle = `window.requestIdleCallback = callback => {
 (window.clerkIdleCallbacks ||= []).push(callback); return window.clerkIdleCallbacks.length;
}; window.startClerk = () => window.clerkIdleCallbacks.splice(0).forEach(callback => callback());`

// Include position AND size of neighbors, the bar, and the account slot. A
// stable slot alone would miss a growing header or a shrinking search field.
const clerkPlaceholderGeometry = `(() => [...document.querySelectorAll(
 '.app-header, .app-header > .logo, .app-header-back, .app-header-title, .app-header-actions > *, [data-gallery-query], .gallery-filter-trigger, .account-control'
)].map(el => ({name:el.getAttribute('aria-label') || el.className.baseVal || el.className,
 box:el.getBoundingClientRect().toJSON()})))()`

func TestClerkPlaceholderBrowser(t *testing.T) {
	app := clerkPlaceholderApp(t)
	browser, err := chromium.New(chromiumtest.Start(t))
	require.NoError(t, err)
	for _, width := range []int64{1280, 768, 767, 390, 320} {
		controlSize := 32
		if width < 768 {
			controlSize = 44
		}
		for _, theme := range []string{"light", "dark"} {
			t.Run(fmt.Sprintf("%d/%s", width, theme), func(t *testing.T) {
				ctx, cancel, err := browser.Tab(context.Background())
				require.NoError(t, err)
				defer cancel()
				ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
				defer cancel()
				_, err = chromedp.Call(ctx, page.AddScriptToEvaluateOnNewDocument, page.AddScriptToEvaluateOnNewDocumentParams{Source: clerkPlaceholderHoldIdle})
				require.NoError(t, err)
				_, err = chromedp.Call(ctx, emulation.SetDeviceMetricsOverride, emulation.SetDeviceMetricsOverrideParams{Width: width, Height: 844, DeviceScaleFactor: 1, Mobile: width < 768})
				require.NoError(t, err)
				_, err = chromedp.Call(ctx, emulation.SetEmulatedMedia, emulation.SetEmulatedMediaParams{Features: []*emulation.MediaFeature{{Name: "prefers-color-scheme", Value: theme}}})
				require.NoError(t, err)
				for _, path := range []string{"/", "/settings"} {
					t.Run(path, func(t *testing.T) {
						run := func(js string) {
							t.Helper()
							require.NoError(t, chromedp.Do(ctx, chromedp.Evaluate[chromedp.Void](js)))
						}
						poll := func(js string) {
							t.Helper()
							require.NoError(t, chromedp.Do(ctx, chromedp.Poll[chromedp.Void](js, chromedp.WithPollingTimeout(5*time.Second))))
						}
						check := func(js string) {
							t.Helper()
							ok, err := chromedp.Run(ctx, chromedp.Evaluate[bool](js))
							require.NoError(t, err)
							require.True(t, ok, js)
						}
						require.NoError(t, chromedp.Do(ctx, chromedp.Navigate(app.URL+path)))
						poll(`window.clerkIdleCallbacks?.length > 0`)
						run(`window.account = document.querySelector('.account-control'); window.mount = account.querySelector('[data-clerk-target="mount"]'); window.placeholder = account.querySelector('.account-control-placeholder');`)
						check(`!window.Clerk && !document.querySelector('script[src*="/npm/"]') && account.dataset.clerkState === 'loading' && account.getAttribute('aria-busy') === 'true' && mount.inert && placeholder.getAttribute('aria-hidden') === 'true' && !placeholder.matches('a,button,[tabindex]') && account.querySelector('[role="status"]').textContent === 'Loading your account…'`)
						check(fmt.Sprintf(`account.getBoundingClientRect().width === %[1]d && account.getBoundingClientRect().height === %[1]d && placeholder.getBoundingClientRect().width === %[1]d && placeholder.getBoundingClientRect().height === %[1]d && getComputedStyle(placeholder).display !== 'none'`, controlSize))
						run(`window.before = ` + clerkPlaceholderGeometry)
						checkStable := func() {
							t.Helper()
							check(`JSON.stringify(before) === JSON.stringify(` + clerkPlaceholderGeometry + `) && document.documentElement.scrollWidth <= innerWidth`)
						}
						run(`startClerk()`)
						poll(`window.finishReload != null`)
						check(`clerkCalls.reload === 1 && clerkCalls.mount === 0 && account.dataset.clerkState === 'loading' && getComputedStyle(placeholder).display !== 'none'`)
						checkStable()
						run(`finishReload()`)
						poll(`window.renderButton != null`)
						check(`mount.firstElementChild != null && account.dataset.clerkState === 'loading' && mount.inert && getComputedStyle(placeholder).display !== 'none'`)
						checkStable()
						run(`renderButton()`)
						check(`account.dataset.clerkState === 'loading' && mount.inert && getComputedStyle(placeholder).display !== 'none'`)
						checkStable()
						run(`enableButton()`)
						poll(`account.dataset.clerkState === 'ready'`)
						checkStable()
						check(`account.getAttribute('aria-busy') === 'false' && !mount.inert && mount.getAttribute('aria-hidden') === 'false' && getComputedStyle(placeholder).display === 'none' && account.querySelector('[role="status"]').textContent === 'Account menu ready.' && account.querySelector('[data-clerk-target="recovery"]').hidden`)
						check(fmt.Sprintf(`mount.querySelector('button').getBoundingClientRect().width === %[1]d && mount.querySelector('button').getBoundingClientRect().height === %[1]d`, controlSize))
						require.NoError(t, chromedp.Do(ctx, chromedp.Click(chromedp.CSS(`.cl-userButtonTrigger`))))
						check(`window.accountClicks === 1 && document.activeElement === mount.querySelector('button')`)
					})
				}
			})
		}
	}
}

func TestClerkPlaceholderFailureBrowser(t *testing.T) {
	app := clerkPlaceholderApp(t)
	browser, err := chromium.New(chromiumtest.Start(t))
	require.NoError(t, err)
	for _, width := range []int64{1280, 390} {
		for _, failure := range []string{"load-failure", "reload-failure", "signed-out", "reload-timeout", "render-timeout"} {
			t.Run(fmt.Sprintf("%d/%s", width, failure), func(t *testing.T) {
				ctx, cancel, err := browser.Tab(context.Background())
				require.NoError(t, err)
				defer cancel()
				ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
				defer cancel()
				_, err = chromedp.Call(ctx, page.AddScriptToEvaluateOnNewDocument, page.AddScriptToEvaluateOnNewDocumentParams{Source: clerkPlaceholderHoldIdle})
				require.NoError(t, err)
				require.NoError(t, chromedp.Do(ctx, chromedp.EmulateViewport(width, 844), chromedp.Navigate(app.URL+"/?account-test="+failure), chromedp.Poll[chromedp.Void](`window.clerkIdleCallbacks?.length > 0`), chromedp.Evaluate[chromedp.Void](`window.before = `+clerkPlaceholderGeometry+`; startClerk();`)))
				if failure == "render-timeout" {
					require.NoError(t, chromedp.Do(ctx, chromedp.Poll[chromedp.Void](`window.finishReload`), chromedp.Evaluate[chromedp.Void](`finishReload()`)))
				}
				require.NoError(t, chromedp.Do(ctx, chromedp.Poll[chromedp.Void](`document.querySelector('.account-control').dataset.clerkState === 'error'`, chromedp.WithPollingTimeout(20*time.Second))))
				ok, err := chromedp.Run(ctx, chromedp.Evaluate[bool](`(() => {
				 const account = document.querySelector('.account-control'), status = account.querySelector('[role="status"]'), recovery = account.querySelector('[data-clerk-target="recovery"]');
				 const feedback = account.querySelector('.account-control-feedback').getBoundingClientRect();
				 return JSON.stringify(before) === JSON.stringify(`+clerkPlaceholderGeometry+`) &&
				  account.getAttribute('aria-busy') === 'false' && account.querySelector('[data-clerk-target="mount"]').inert &&
				  !status.classList.contains('sr-only') && status.textContent.length > 0 && !recovery.hidden &&
				  recovery.querySelector('a').getAttribute('href') === '/settings' &&
				  feedback.top >= document.querySelector('header').getBoundingClientRect().bottom && feedback.left >= 0 && feedback.right <= innerWidth && feedback.bottom <= innerHeight && document.documentElement.scrollWidth <= innerWidth;
				})()`))
				require.NoError(t, err)
				if !ok {
					detail, detailErr := chromedp.Run(ctx, chromedp.Evaluate[string](`JSON.stringify({before,after:`+clerkPlaceholderGeometry+`,feedback:document.querySelector('.account-control-feedback').getBoundingClientRect().toJSON(),header:document.querySelector('header').getBoundingClientRect().toJSON(),scrollWidth:document.documentElement.scrollWidth,viewport:{width:innerWidth,height:innerHeight}})`))
					require.NoError(t, detailErr)
					t.Log(detail)
				}
				require.True(t, ok, "failure feedback remains visible, accessible and outside the unchanged header layout")
				if failure == "reload-timeout" {
					require.NoError(t, chromedp.Do(ctx, chromedp.Evaluate[chromedp.Void](`finishReload()`)))
					calls, err := chromedp.Run(ctx, chromedp.Evaluate[int](`clerkCalls.mount`))
					require.NoError(t, err)
					require.Zero(t, calls, "a permission reload completing after timeout must not mount the button")
				}
				if failure == "render-timeout" {
					calls, err := chromedp.Run(ctx, chromedp.Evaluate[int](`clerkCalls.unmount`))
					require.NoError(t, err)
					require.Equal(t, 1, calls, "a render timeout unmounts the stalled provider UI")
				}
				// Recovery is keyboard reachable. Retry reloads the current page;
				// replace only its test scenario in history before activating it.
				require.NoError(t, chromedp.Do(ctx, chromedp.Focus(chromedp.CSS(`.account-control-recovery button`)), chromedp.KeyEvent(kb.Tab)))
				ok, err = chromedp.Run(ctx, chromedp.Evaluate[bool](`document.activeElement.matches('.account-control-recovery a')`))
				require.NoError(t, err)
				require.True(t, ok)
				require.NoError(t, chromedp.Do(ctx,
					chromedp.Evaluate[chromedp.Void](`history.replaceState(null,'','/?account-test=ready')`),
					chromedp.Focus(chromedp.CSS(`.account-control-recovery button`)),
				))
				_, err = chromedp.RunResponse(ctx, chromedp.KeyEvent(kb.Enter))
				require.NoError(t, err)
				require.NoError(t, chromedp.Do(ctx,
					chromedp.Poll[chromedp.Void](`window.clerkIdleCallbacks?.length > 0 && !window.Clerk`),
					chromedp.Evaluate[chromedp.Void](`startClerk()`),
					chromedp.Poll[chromedp.Void](`document.querySelector('.account-control').dataset.clerkState === 'ready'`),
				))
			})
		}
	}
}

func TestClerkPlaceholderReconnectBrowser(t *testing.T) {
	app := clerkPlaceholderApp(t)
	browser, err := chromium.New(chromiumtest.Start(t))
	require.NoError(t, err)
	ctx, cancel, err := browser.Tab(context.Background())
	require.NoError(t, err)
	defer cancel()
	ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	require.NoError(t, chromedp.Do(ctx, chromedp.Navigate(app.URL), chromedp.Poll[chromedp.Void](`window.reloads?.length === 1`),
		chromedp.Evaluate[chromedp.Void](`window.account = document.querySelector('.account-control'); window.accountParent = account.parentNode; account.remove();`),
	))
	// Let Stimulus process removal before reconnecting the same element.
	require.NoError(t, chromedp.Do(ctx, chromedp.Evaluate[chromedp.Void](`accountParent.append(account)`), chromedp.Poll[chromedp.Void](`reloads.length === 2`),
		chromedp.Evaluate[chromedp.Void](`reloads[0]()`),
	))
	ok, err := chromedp.Run(ctx, chromedp.Evaluate[bool](`clerkCalls.mount === 0 && account.dataset.clerkState === 'loading'`))
	require.NoError(t, err)
	require.True(t, ok, "a stale permission reload must not mount into the new connection")
	require.NoError(t, chromedp.Do(ctx, chromedp.Evaluate[chromedp.Void](`reloads[1]()`), chromedp.Poll[chromedp.Void](`window.renderButton`),
		// Disconnect while the trigger is still disabled. Its late render must
		// not reveal the account control in the subsequent connection.
		chromedp.Evaluate[chromedp.Void](`renderButton(); account.remove()`), chromedp.Poll[chromedp.Void](`clerkCalls.unmount === 1`),
		chromedp.Evaluate[chromedp.Void](`accountParent.append(account)`), chromedp.Poll[chromedp.Void](`reloads.length === 3`),
		chromedp.Evaluate[chromedp.Void](`enableButton()`),
	))
	ok, err = chromedp.Run(ctx, chromedp.Evaluate[bool](`account.dataset.clerkState === 'loading' && account.querySelector('[data-clerk-target="mount"]').inert`))
	require.NoError(t, err)
	require.True(t, ok, "a disconnected render must not complete the new loading state")
	require.NoError(t, chromedp.Do(ctx,
		chromedp.Evaluate[chromedp.Void](`reloads[2]()`), chromedp.Poll[chromedp.Void](`clerkCalls.mount === 2`),
		chromedp.Evaluate[chromedp.Void](`renderButton(); enableButton()`), chromedp.Poll[chromedp.Void](`account.dataset.clerkState === 'ready'`),
		chromedp.Evaluate[chromedp.Void](`account.remove()`), chromedp.Poll[chromedp.Void](`clerkCalls.unmount === 2`),
	))
}
