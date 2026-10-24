package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/momja/Exhibit/internal/auth"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/require"
)

type hostedLoginStub struct{}

func (hostedLoginStub) Middleware(next http.Handler) http.Handler { return next }
func (hostedLoginStub) Subject(r *http.Request) string            { return r.Header.Get("Test-Clerk-Subject") }
func (hostedLoginStub) Email(context.Context, string) (string, error) {
	return "person@example.com", nil
}

func TestClerkHostedMode(t *testing.T) {
	cred, err := auth.NewCredential("selfhost", "$2a$10$N9qo8uLOickgx2ZMRZoMy.Mrq4H/M2dYs6qVu0Y0lJ4YFjIsWJc.1W")
	require.NoError(t, err)
	ro, st := newLoginTestRouter(t, &stubProvider{}, cred, func(c *Config) {
		c.Clerk = hostedLoginStub{}
		c.ClerkPublishableKey = "pk_test_example"
		c.ClerkFrontendAPI = "https://example.clerk.accounts.dev"
		c.AuthToken = ""
	})
	request := func(method, path, subject string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "https://app.test"+path, nil)
		if subject != "" {
			r.Header.Set("Test-Clerk-Subject", subject)
		}
		w := httptest.NewRecorder()
		ro.ServeHTTP(w, r)
		return w
	}
	require.False(t, ro.localLoginEnabled())
	require.False(t, ro.identityEnabled())
	for _, path := range []string{"/auth/login", "/auth/signup"} {
		w := request("GET", path, "")
		require.Equal(t, 200, w.Code)
		require.Contains(t, w.Body.String(), "clerk.js")
		require.NotContains(t, w.Body.String(), "action=\"/auth/local\"")
	}
	require.Equal(t, 404, request("POST", "/auth/local", "").Code)
	require.Equal(t, 404, request("GET", "/auth/sso", "").Code)
	require.Equal(t, 302, request("GET", "/", "").Code)
	require.Equal(t, 401, request("GET", "/api/artifacts", "").Code)
	first := request("GET", "/auth/clerk/ready", "clerk:user_one")
	require.Equal(t, 204, first.Code)
	second := request("GET", "/auth/clerk/ready", "clerk:user_two")
	require.Equal(t, 204, second.Code)
	one, err := st.GetUserByExternalID(context.Background(), "clerk:user_one")
	require.NoError(t, err)
	two, err := st.GetUserByExternalID(context.Background(), "clerk:user_two")
	require.NoError(t, err)
	require.NotEqual(t, one.ID, two.ID)
	require.Equal(t, 200, request("GET", "/", "clerk:user_one").Code)
	// Both identities have the same email, but distinct libraries and permissions.
	require.NoError(t, st.SetUserDisabled(context.Background(), two.ID, true))
	require.Equal(t, 403, request("GET", "/auth/clerk/ready", "clerk:user_two").Code)
	require.Equal(t, 401, request("GET", "/api/artifacts", "clerk:user_two").Code)
	// Hosted mode never accepts old self-hosted session cookies.
	require.NoError(t, st.CreateSession(context.Background(), &store.Session{ID: "old", UserID: one.ID, ExpiresAt: time.Now().Add(time.Hour)}))
	r := httptest.NewRequest("GET", "/api/artifacts", nil)
	r.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "old"})
	w := httptest.NewRecorder()
	ro.ServeHTTP(w, r)
	require.Equal(t, 401, w.Code)
}

func TestClerkLoginRedirectsToSecurePublicOrigin(t *testing.T) {
	ro, _ := newLoginTestRouter(t, nil, nil, func(c *Config) { c.Clerk = hostedLoginStub{} })
	r := httptest.NewRequest("GET", "http://old-host:8096/auth/signup?next=%2F", nil)
	w := httptest.NewRecorder()
	ro.ServeHTTP(w, r)
	require.Equal(t, http.StatusSeeOther, w.Code)
	require.Equal(t, "https://app.test/auth/signup?next=%2F", w.Header().Get("Location"))
}
