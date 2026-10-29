package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/momja/Exhibit/internal/auth"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/require"
)

type hostedLoginStub struct{}

func (hostedLoginStub) Middleware(next http.Handler) http.Handler { return next }
func (hostedLoginStub) Subject(r *http.Request) string            { return r.Header.Get("Test-Clerk-Subject") }
func (hostedLoginStub) SessionExpiresIn(r *http.Request) time.Duration {
	if r.Header.Get("Test-Clerk-Subject") == "" {
		return 0
	}
	return 42 * time.Second
}
func (hostedLoginStub) Identity(r *http.Request) (auth.HostedIdentity, error) {
	return auth.HostedIdentity{Subject: r.Header.Get("Test-Clerk-Subject"), Email: "person@example.com", SessionCreatedAt: time.Now().UnixMilli()}, nil
}
func (hostedLoginStub) HandshakeRedirect(w http.ResponseWriter, r *http.Request) bool {
	if r.Header.Get("Test-Clerk-Handshake") == "" {
		return false
	}
	http.Redirect(w, r, "https://example.clerk.accounts.dev/v1/client/handshake", http.StatusTemporaryRedirect)
	return true
}
func (hostedLoginStub) ResolveEmail(context.Context, string) (string, error)  { return "", nil }
func (hostedLoginStub) RevokeSessions(context.Context, string) (int64, error) { return 0, nil }
func (hostedLoginStub) DeleteUser(context.Context, string) error              { return nil }

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
	// Exh-8409: a lapsed hosted session renews through the handshake when the
	// provider can, before falling back to the login page; fragments and the
	// API never take a cross-site redirect.
	for _, path := range []string{"/", "/artifacts/a1", "/partials/gallery-grid", "/api/artifacts"} {
		r := httptest.NewRequest("GET", "https://app.test"+path, nil)
		r.Header.Set("Test-Clerk-Handshake", "1")
		if path == "/partials/gallery-grid" {
			r.Header.Set("HX-Request", "true")
		}
		w := httptest.NewRecorder()
		ro.ServeHTTP(w, r)
		if strings.HasPrefix(path, "/partials/") || strings.HasPrefix(path, "/api/") {
			require.Equal(t, 401, w.Code, path)
			continue
		}
		require.Equal(t, http.StatusTemporaryRedirect, w.Code, path)
		require.True(t, strings.HasPrefix(w.Header().Get("Location"), "https://example.clerk.accounts.dev/v1/client/handshake"), path)
	}
	first := request("GET", "/auth/clerk/ready", "clerk:user_one")
	require.Equal(t, 204, first.Code)
	second := request("GET", "/auth/clerk/ready", "clerk:user_two")
	require.Equal(t, 204, second.Code)
	one, err := st.GetUserByExternalID(context.Background(), "clerk:user_one")
	require.NoError(t, err)
	two, err := st.GetUserByExternalID(context.Background(), "clerk:user_two")
	require.NoError(t, err)
	require.NotEqual(t, one.ID, two.ID)
	page := request("GET", "/", "clerk:user_one")
	require.Equal(t, 200, page.Code)
	// Exh-g0fx: the page learns how long its token has left from the server,
	// whose clock the token is checked against.
	require.Contains(t, page.Body.String(), `<meta name="clerk-session-expires-in" content="42000">`)
	require.Contains(t, request("GET", "/auth/login", "").Body.String(), `<meta name="clerk-session-expires-in" content="0">`)
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

type emailHostedLogin struct {
	hostedLoginStub
	email string
}

func (h *emailHostedLogin) Identity(r *http.Request) (auth.HostedIdentity, error) {
	identity, err := h.hostedLoginStub.Identity(r)
	identity.Email = h.email
	return identity, err
}
func (h *emailHostedLogin) ResolveEmail(_ context.Context, email string) (string, error) {
	if email == h.email {
		return "clerk:member", nil
	}
	return "", nil
}
func TestClerkEmailChangeUpdatesProfileAndShareLookup(t *testing.T) {
	h := &emailHostedLogin{email: "old@example.test"}
	ro, st := newLoginTestRouter(t, nil, nil, func(c *Config) { c.Clerk = h })
	req := httptest.NewRequest("GET", "https://app.test/auth/clerk/ready", nil)
	req.Header.Set("Test-Clerk-Subject", "clerk:member")
	oldID, ok := ro.clerkSessionUser(req)
	require.True(t, ok)
	h.email = "new@example.test"
	// Recipient resolution must already use the new address before the recipient
	// revisits Exhibit and before the cached local profile is refreshed.
	_, err := ro.resolveRecipient(context.Background(), "old@example.test")
	require.ErrorIs(t, err, store.ErrNotFound)
	recipient, err := ro.resolveRecipient(context.Background(), h.email)
	require.NoError(t, err)
	require.Equal(t, oldID, recipient.ID)
	id, ok := ro.clerkSessionUser(req)
	require.True(t, ok)
	require.Equal(t, oldID, id)
	u, err := st.GetUser(context.Background(), id)
	require.NoError(t, err)
	require.Equal(t, h.email, u.Email)
}

func TestClerkUnifiedAccountPages(t *testing.T) {
	ro, st := newLoginTestRouter(t, nil, nil, func(c *Config) {
		c.Clerk = hostedLoginStub{}
		c.ClerkPublishableKey = "pk_test_example"
		c.ClerkFrontendAPI = "https://example.clerk.accounts.dev"
		c.AuthToken = ""
	})
	get := func(path, subject string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", path, nil)
		r.Header.Set("Test-Clerk-Subject", subject)
		w := httptest.NewRecorder()
		ro.ServeHTTP(w, r)
		return w
	}
	require.Equal(t, 204, get("/auth/clerk/ready", "clerk:admin").Code)
	require.Equal(t, 204, get("/auth/clerk/ready", "clerk:member").Code)
	member, err := st.GetUserByExternalID(context.Background(), "clerk:member")
	require.NoError(t, err)
	require.NoError(t, st.PutArtifact(context.Background(), &store.Artifact{ID: "account-artifact", OwnerID: member.ID, Title: "Member", SourceBlobID: "account-blob", Tier: store.Tier1, CreatedAt: time.Now(), UpdatedAt: time.Now()}))
	require.NoError(t, st.RecordBlobSize(context.Background(), "account-blob", 12345))
	page := get("/", "clerk:member")
	require.Equal(t, 200, page.Code)
	require.Equal(t, 1, strings.Count(page.Body.String(), `data-clerk-mode-value="user"`))
	require.NotContains(t, page.Body.String(), `aria-label="Your account" title="Your account"`)

	page = get("/settings", "clerk:member")
	require.Equal(t, 200, page.Code)
	require.Equal(t, "no-store", page.Header().Get("Cache-Control"))
	require.Contains(t, page.Body.String(), "Settings — Exhibit")
	require.Contains(t, page.Body.String(), "Manage profile")
	require.Contains(t, page.Body.String(), "12.1 KiB")
	require.Contains(t, page.Body.String(), "1 artifact")
	require.Contains(t, page.Body.String(), "sign-in methods stored with Clerk")
	require.Contains(t, page.Body.String(), "Delete my account permanently")
	require.NotContains(t, page.Body.String(), "Signing in again will still work")
	require.NotContains(t, get("/settings", "clerk:admin").Body.String(), "12.1 KiB")
	require.Contains(t, get("/settings", "clerk:admin").Body.String(), "You are the only account")
	require.Equal(t, 200, get("/profile", "clerk:member").Code)
	require.Equal(t, 302, get("/settings", "").Code)

}
