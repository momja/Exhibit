package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/require"
)

func TestHostedAdministrationRejectsUnusableLocalCredentials(t *testing.T) {
	ro, st := newLoginTestRouter(t, nil, nil, func(c *Config) { c.Clerk = hostedLoginStub{} })
	ctx := context.Background()
	admin, err := st.UpsertUser(ctx, "clerk:admin", "admin@example.test")
	require.NoError(t, err)
	legacy, err := st.CreateLocalUser(ctx, store.NewLocalUser{ExternalID: "local:legacy", Email: "legacy", PasswordHash: testHash(t, goodPass)})
	require.NoError(t, err)
	require.NoError(t, st.SetUserAdmin(ctx, legacy.ID, true))
	request := func(method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "https://app.test"+path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Test-Clerk-Subject", "clerk:admin")
		w := httptest.NewRecorder()
		ro.ServeHTTP(w, r)
		return w
	}
	require.Equal(t, http.StatusForbidden, request("POST", "/api/admin/users", `{"username":"new","password":"long-enough-password","is_admin":true}`).Code)
	require.Equal(t, http.StatusForbidden, request("PATCH", userPath(legacy.ID), `{"password":"replacement-password","is_admin":false}`).Code)
	after, err := st.GetUser(ctx, legacy.ID)
	require.NoError(t, err)
	require.True(t, after.IsAdmin, "password refusal must precede all writes")
	require.Equal(t, http.StatusBadRequest, request("PATCH", userPath(legacy.ID), `{"is_admin":true}`).Code)
	for _, body := range []string{`{"is_admin":false}`, `{"disabled":true}`} {
		require.Equal(t, http.StatusConflict, request("PATCH", userPath(admin.ID), body).Code)
	}
	page := request("GET", "/admin/users", "")
	require.Equal(t, http.StatusOK, page.Code)
	require.NotContains(t, page.Body.String(), `id="create-user"`)
	require.NotContains(t, page.Body.String(), `data-action="password"`)
	require.Contains(t, page.Body.String(), "Unavailable in hosted mode")
	last, err := ro.isLastEnabledHostedAdmin(ctx, admin)
	require.NoError(t, err)
	require.True(t, last)
}
