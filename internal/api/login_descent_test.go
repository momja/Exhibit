package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func getLogin(ro *Router, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", "/auth/login", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	w := httptest.NewRecorder()
	ro.ServeHTTP(w, req)
	return w
}

// The sign-in descent (Exh-vsw0) plays once: the first visit animates and
// records that, a repeat visit shows the final frame, and signing out re-arms
// it. The page stays scriptless, and layer art never appears in its markup.
func TestLoginDescentPlaysOncePerSignOut(t *testing.T) {
	ro, _ := newLocalLoginRouter(t)

	first := getLogin(ro)
	require.Equal(t, 200, first.Code)
	assert.Contains(t, first.Body.String(), `class="login-page login-painted login-descend"`)
	assert.Contains(t, first.Body.String(), `<template>`)
	assert.Contains(t, first.Body.String(), `login-descent.js`)
	seen := cookiesFrom(first)[descentCookieName]
	require.NotNil(t, seen, "the first visit records that the descent was shown")
	assert.True(t, seen.HttpOnly)
	assert.Equal(t, http.SameSiteLaxMode, seen.SameSite)
	assert.Equal(t, "/", seen.Path)
	assert.Empty(t, seen.Domain)
	assert.Equal(t, "1", seen.Value, "the cookie carries no information about the visitor")

	repeat := getLogin(ro, seen)
	assert.Contains(t, repeat.Body.String(), `class="login-page login-painted"`)
	assert.NotContains(t, repeat.Body.String(), "login-descend")
	assert.NotContains(t, repeat.Body.String(), "login-layer")
	assert.NotContains(t, repeat.Body.String(), "<script", "a repeat visit loads no script and no layer art")
	assert.Nil(t, cookiesFrom(repeat)[descentCookieName], "nothing to re-record")

	// A rejected password re-renders the page; it must not replay the descent.
	req := httptest.NewRequest("POST", "/auth/local", strings.NewReader("username=nobody&password=wrong"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(seen)
	failed := httptest.NewRecorder()
	ro.ServeHTTP(failed, req)
	assert.NotContains(t, failed.Body.String(), "login-descend")

	session := runLocalLogin(t, ro)
	out := httptest.NewRequest("POST", "/auth/logout", nil)
	out.AddCookie(session)
	out.AddCookie(seen)
	loggedOut := httptest.NewRecorder()
	ro.ServeHTTP(loggedOut, out)
	cleared := cookiesFrom(loggedOut)[descentCookieName]
	require.NotNil(t, cleared, "sign-out clears the marker")
	assert.Less(t, cleared.MaxAge, 0)

	again := getLogin(ro) // the browser dropped the cleared cookie
	assert.Contains(t, again.Body.String(), "login-descend")
}

// The Secure attribute follows APP_ORIGIN like the session cookie's.
func TestLoginDescentCookieSecureFollowsOrigin(t *testing.T) {
	ro, _ := newLocalLoginRouter(t)
	ro.cfg.AppOrigin = "https://exhibit.example"
	c := cookiesFrom(getLogin(ro))[descentCookieName]
	require.NotNil(t, c)
	assert.True(t, c.Secure)
}

// Hosted mode (Exh-vsw0): Clerk's sign-out lands on GET /auth/logout, which
// clears the marker and plays nothing, while the sign-in and sign-up pages set
// the same constant marker the local page does. These are the GET routes the
// CSRF table declares as mutating (csrf_test.go), so their writes are pinned
// here: the marker and nothing else, with the session cookie's attributes.
func TestHostedDescentCookieOnlyTouchesTheMarker(t *testing.T) {
	cfg := newTestRouter(t).cfg
	cfg.Clerk = hostedLoginStub{}
	cfg.ClerkPublishableKey = "pk_test_example"
	cfg.ClerkFrontendAPI = "https://example.clerk.accounts.dev"
	ro := NewRouter(cfg)

	get := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		ro.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		return w
	}
	for _, path := range []string{"/auth/login", "/auth/signup"} {
		w := get(path)
		require.Equal(t, 200, w.Code, path)
		assert.Contains(t, w.Body.String(), "login-descend", path)
		cookies := cookiesFrom(w)
		require.Len(t, cookies, 1, "%s sets the marker and nothing else", path)
		c := cookies[descentCookieName]
		require.NotNil(t, c, path)
		assert.Equal(t, "1", c.Value)
		assert.True(t, c.HttpOnly)
		assert.Equal(t, http.SameSiteLaxMode, c.SameSite)
		assert.Empty(t, c.Domain)
	}

	out := get("/auth/logout")
	require.Equal(t, 200, out.Code)
	assert.NotContains(t, out.Body.String(), "login-descend")
	cookies := cookiesFrom(out)
	require.Len(t, cookies, 1, "hosted sign-out clears the marker and nothing else")
	assert.Less(t, cookies[descentCookieName].MaxAge, 0)
}
