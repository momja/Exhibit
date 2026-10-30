package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/momja/Exhibit/internal/chromium"
	"github.com/momja/Exhibit/internal/chromium/chromiumtest"
	"github.com/stretchr/testify/require"
)

// Stub only the provider UI boundary. Use the real Settings page, authenticated
// routes, Stimulus, and deletion to exercise the single account-menu entry point.
func TestClerkAccountBrowser(t *testing.T) {
	t.Run("complete", func(t *testing.T) { testClerkAccountBrowser(t, false) })
	t.Run("provider retry", func(t *testing.T) { testClerkAccountBrowser(t, true) })
}

func testClerkAccountBrowser(t *testing.T, pending bool) {
	browser, err := chromium.New(chromiumtest.Start(t))
	require.NoError(t, err)
	ctx, cancel, err := browser.Tab(context.Background())
	require.NoError(t, err)
	defer cancel()
	ctx, cancel = context.WithTimeout(ctx, 40*time.Second)
	defer cancel()
	hosted := &deletionHostedLogin{createdAt: time.Now().Add(-time.Hour).UnixMilli()}
	if pending {
		hosted.deleteErr = errors.New("provider unavailable")
	}
	ro, st := newLoginTestRouter(t, nil, nil, func(c *Config) {
		c.Clerk = hosted
		c.AuthToken = ""
		c.ClerkPublishableKey = "pk_test_example"
		c.ClerkFrontendAPI = "https://example.clerk.accounts.dev"
	})
	_, err = st.UpsertHostedUser(ctx, "clerk:admin", "admin@example.test", hosted.createdAt)
	require.NoError(t, err)
	_, err = st.UpsertHostedUser(ctx, "clerk:member", "member@example.test", hosted.createdAt)
	require.NoError(t, err)

	// Seed through the actual write route so storage and deletion exercise stored
	// bytes, reference accounting, and the local blob backend together.
	const artifactBody = "<!doctype html><title>Weekly tracker</title><p>Production settings integration</p>"
	payload, err := json.Marshal(map[string]any{"title": "Weekly tracker", "body": artifactBody, "network_allowlist": []string{}})
	require.NoError(t, err)
	request := httptest.NewRequest("POST", "/api/artifacts", strings.NewReader(string(payload)))
	request.Header.Set("Test-Clerk-Subject", "clerk:member")
	request.Header.Set("Content-Type", "application/json")
	created := httptest.NewRecorder()
	ro.ServeHTTP(created, request)
	require.Equal(t, http.StatusCreated, created.Code, created.Body.String())
	var artifact createArtifactResponse
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &artifact))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/gallery/clerk-client.js") {
			w.Header().Set("Content-Type", "text/javascript")
			fmt.Fprint(w, clerkAccountBrowserProvider)
			return
		}
		if r.URL.Path == "/auth/logout" {
			fmt.Fprint(w, "Signed out after deletion")
			return
		}
		r.Header.Set("Test-Clerk-Subject", "clerk:member")
		ro.ServeHTTP(w, r)
	}))
	defer srv.Close()
	click := func(selector string) chromedp.Action[chromedp.Void] {
		return chromedp.Evaluate[chromedp.Void](fmt.Sprintf(`document.querySelector(%q).click()`, selector))
	}
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

	run(chromedp.EmulateViewport(1100, 1200), chromedp.Navigate(srv.URL), chromedp.WaitVisible(chromedp.CSS(`#test-account-button`)), click(`#test-account-button`))
	check(`document.querySelectorAll('#test-account-button').length === 1 && !document.querySelector('a[aria-label="Settings"]')`)
	run(click(`#test-manage-profile`))
	check(`window.profileOpens === 1 && window.profileOptions === undefined`)
	_, err = chromedp.RunResponse(ctx, click(`#test-settings`))
	require.NoError(t, err)
	run(chromedp.WaitVisible(chromedp.CSS(`#delete-account`)))
	check(`location.pathname === '/settings' && document.title === 'Settings — Exhibit'`)
	check(fmt.Sprintf(`document.querySelector('#storage').textContent.includes(%q) && document.querySelector('#storage').textContent.includes('1 artifact')`, fmt.Sprintf("%d B", len(artifactBody))))
	run(chromedp.WaitVisible(chromedp.CSS(`#test-account-button`)))
	check(`document.querySelectorAll('[data-clerk-mode-value="user"]').length === 1 && document.querySelector('header #test-account-button') != null`)

	check(`getComputedStyle(document.querySelector('[data-account-deletion-target="finish"]')).display === 'none'`)
	run(chromedp.Evaluate[chromedp.Void](fmt.Sprintf(`import(%q).then(({application}) => { window.accountApplication = application; }).catch(error => { window.accountImportError = error.stack; })`, assetURL("gallery/controllers.js"))),
		chromedp.Poll[chromedp.Void](`window.accountApplication?.getControllerForElementAndIdentifier(document.querySelector('[data-clerk-mode-value="manage"]'), 'clerk') != null`),
		click(`[data-action="clerk#openProfile"]`),
	)
	run(chromedp.Poll[chromedp.Void](`window.profileOpens === 1`))
	check(`window.profileOpens === 1 && window.profileOptions === undefined`)
	run(click(`#delete-account`))
	check(`document.activeElement.id === 'delete-phrase' && document.querySelector('#delete-confirm-go').disabled`)
	run(chromedp.SendKeys(chromedp.CSS(`#delete-phrase`), "delete my library"))
	check(`!document.querySelector('#delete-confirm-go').disabled`)
	run(click(`#delete-cancel`))
	check(`document.querySelector('#delete-phrase').value === '' && document.querySelector('#delete-confirm').hidden`)
	// Capture the production layout; only the provider boundary is stubbed.
	capture := func(name string) {
		if dir := os.Getenv("ACCOUNT_SCREENSHOT_DIR"); dir != "" {
			require.NoError(t, os.MkdirAll(dir, 0755))
			shot, err := chromedp.Run(ctx, chromedp.Screenshot(chromedp.CSS(`body`)))
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(dir, name), shot, 0644))
		}
	}

	capture("settings.png")
	if path := os.Getenv("SETTINGS_PREVIEW_HTML"); path != "" {
		html, err := chromedp.Run(ctx, chromedp.Evaluate[string](`document.documentElement.outerHTML`))
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(path, []byte(html), 0644))
	}
	run(chromedp.EmulateViewport(390, 1100))
	check(`document.documentElement.scrollWidth <= innerWidth`)
	capture("settings-mobile.png")
	run(chromedp.EmulateViewport(1100, 1200))
	run(click(`#delete-account`), chromedp.SendKeys(chromedp.CSS(`#delete-phrase`), "delete my library"))
	if !pending {
		_, err := chromedp.RunResponse(ctx, click(`#delete-confirm-go`))
		require.NoError(t, err)
	} else {
		run(click(`#delete-confirm-go`))
	}
	if pending {
		run(chromedp.WaitVisible(chromedp.CSS(`[data-account-deletion-target="finish"]`)))
		check(`document.querySelector('#profile-status').textContent.includes('retry automatically') && document.querySelector('#delete-account').disabled`)
		_, err := chromedp.RunResponse(ctx, click(`[data-account-deletion-target="finish"]`))
		require.NoError(t, err)
	}
	check(`location.pathname === '/auth/logout'`)
	require.Equal(t, []string{"clerk:member"}, hosted.deleted)
	body, err := ro.cfg.Blob.Get(ctx, artifact.Artifact.SourceBlobID)
	if body != nil {
		_ = body.Close()
	}
	require.ErrorIs(t, err, fs.ErrNotExist, "account erasure removes the artifact bytes, not just its rows")
	queued, err := st.PendingHostedIdentityDeletions(ctx)
	require.NoError(t, err)
	if pending {
		require.Equal(t, []string{"clerk:member"}, queued)
	} else {
		require.Empty(t, queued)
	}
}

const clerkAccountBrowserProvider = `
document.documentElement.style.scrollbarGutter = 'stable';
window.profileOpens = 0;
const clerk = {
 isSignedIn:true, user:{reload: async () => {}},
 openUserProfile(options) { window.profileOpens++; window.profileOptions = options; },
 mountUserButton(element, options) {
  const button = document.createElement('button'); button.id = 'test-account-button'; button.setAttribute('aria-label', 'Your account'); button.className = 'btn btn-sec icon-btn'; const glyph = document.createElement('i'); glyph.className = 'ph ph-user-circle'; glyph.setAttribute('aria-hidden', 'true'); button.append(glyph);
  button.onclick = () => {
   if (document.querySelector('#test-account-menu')) return;
   const menu = document.createElement('nav'); menu.id = 'test-account-menu';
   const profile = document.createElement('button'); profile.id = 'test-manage-profile'; profile.textContent = 'Manage profile'; profile.onclick = () => clerk.openUserProfile(); menu.append(profile);
   for (const item of options.customMenuItems) {
    const link = document.createElement('a'); link.id = 'test-settings'; link.textContent = item.label; link.href = item.href; menu.append(link);
   }
   element.append(menu);
  };
  element.append(button);
 },
 unmountUserButton() {},
};
export async function clerkClient() { return clerk; }
export const clerkClientAfterLoad = clerkClient;
`
