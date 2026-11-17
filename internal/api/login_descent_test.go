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
	assert.Contains(t, first.Body.String(), `login-layer-sky`)
	assert.NotContains(t, first.Body.String(), "<script")
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
