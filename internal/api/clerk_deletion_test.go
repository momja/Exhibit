package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/momja/Exhibit/internal/auth"
	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/require"
)

type deletionHostedLogin struct {
	hostedLoginStub
	createdAt int64
	revokeErr error
	revoked   []string
}

func (h *deletionHostedLogin) Identity(r *http.Request) (auth.HostedIdentity, error) {
	return auth.HostedIdentity{Subject: h.Subject(r), Email: "member@example.test", SessionCreatedAt: h.createdAt}, nil
}

func (h *deletionHostedLogin) RevokeSessions(_ context.Context, subject string) (int64, error) {
	h.revoked = append(h.revoked, subject)
	return h.createdAt, h.revokeErr
}

func TestClerkAccountDeletionRevokesSessionsAndFencesReplay(t *testing.T) {
	ctx := context.Background()
	hosted := &deletionHostedLogin{createdAt: time.Now().Add(-time.Hour).UnixMilli()}
	ro, st := newLoginTestRouter(t, nil, nil, func(c *Config) {
		c.Clerk = hosted
		c.AuthToken = ""
	})
	request := func(method, path, subject string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "https://app.test"+path, strings.NewReader(`{"confirm":"delete my library"}`))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Test-Clerk-Subject", subject)
		w := httptest.NewRecorder()
		ro.ServeHTTP(w, r)
		return w
	}
	require.Equal(t, 204, request("GET", "/auth/clerk/ready", "clerk:admin").Code)
	require.Equal(t, 204, request("GET", "/auth/clerk/ready", "clerk:member").Code)
	member, err := st.GetUserByExternalID(ctx, "clerk:member")
	require.NoError(t, err)

	// Refuse the last usable administrator before making any provider change.
	require.Equal(t, 409, request("DELETE", "/api/account", "clerk:admin").Code)
	require.Empty(t, hosted.revoked)

	// A provider failure must preserve the library rather than relying on JS.
	hosted.revokeErr = errors.New("provider unavailable")
	require.Equal(t, 500, request("DELETE", "/api/account", "clerk:member").Code)
	_, err = st.GetUser(ctx, member.ID)
	require.NoError(t, err)

	hosted.revokeErr = nil
	require.Equal(t, 204, request("DELETE", "/api/account", "clerk:member").Code)
	require.Equal(t, []string{"clerk:member", "clerk:member"}, hosted.revoked)
	_, err = st.GetUser(ctx, member.ID)
	require.ErrorIs(t, err, store.ErrNotFound)
	// Stub still presents the old provider session: the database fence alone
	// must stop it, even when its JWT hasn't expired or browser JS never runs.
	require.Equal(t, 401, request("GET", "/api/artifacts", "clerk:member").Code)
	require.Equal(t, 403, request("GET", "/auth/clerk/ready", "clerk:member").Code)
	_, err = st.GetUserByExternalID(ctx, "clerk:member")
	require.ErrorIs(t, err, store.ErrNotFound)

	oldCreatedAt := hosted.createdAt
	hosted.createdAt = time.Now().Add(time.Second).UnixMilli()
	require.Equal(t, 204, request("GET", "/auth/clerk/ready", "clerk:member").Code)
	fresh, err := st.GetUserByExternalID(ctx, "clerk:member")
	require.NoError(t, err)
	require.NotEqual(t, member.ID, fresh.ID)
	hosted.createdAt = oldCreatedAt
	require.Equal(t, 401, request("GET", "/api/artifacts", "clerk:member").Code)
}
