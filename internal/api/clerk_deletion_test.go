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
	deleteErr error
	deleted   []string
	onRevoke  func()
}

func (h *deletionHostedLogin) Identity(r *http.Request) (auth.HostedIdentity, error) {
	return auth.HostedIdentity{Subject: h.Subject(r), Email: "member@example.test", SessionCreatedAt: h.createdAt}, nil
}

func (h *deletionHostedLogin) RevokeSessions(_ context.Context, subject string) (int64, error) {
	h.revoked = append(h.revoked, subject)
	if h.onRevoke != nil {
		h.onRevoke()
	}
	return h.createdAt, h.revokeErr
}

func (h *deletionHostedLogin) DeleteUser(_ context.Context, subject string) error {
	h.deleted = append(h.deleted, subject)
	return h.deleteErr
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
	require.Empty(t, hosted.deleted)

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

	require.Equal(t, []string{"clerk:member"}, hosted.deleted)
	pending, err := st.PendingHostedIdentityDeletions(ctx)
	require.NoError(t, err)
	require.Empty(t, pending)
	hosted.createdAt = time.Now().Add(time.Second).UnixMilli()
	require.Equal(t, 403, request("GET", "/auth/clerk/ready", "clerk:member").Code)
}

func TestClerkIdentityDeletionFailureIsDurableAndRetried(t *testing.T) {
	ctx := context.Background()
	hosted := &deletionHostedLogin{createdAt: time.Now().Add(-time.Hour).UnixMilli(), deleteErr: errors.New("provider unavailable")}
	ro, st := newLoginTestRouter(t, nil, nil, func(c *Config) { c.Clerk = hosted; c.AuthToken = "" })
	_, err := st.UpsertHostedUser(ctx, "clerk:admin", "admin@example.test", hosted.createdAt)
	require.NoError(t, err)
	member, err := st.UpsertHostedUser(ctx, "clerk:member", "member@example.test", hosted.createdAt)
	require.NoError(t, err)
	r := httptest.NewRequest("DELETE", "https://app.test/api/account", strings.NewReader(`{"confirm":"delete my library"}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Test-Clerk-Subject", "clerk:member")
	w := httptest.NewRecorder()
	ro.ServeHTTP(w, r)
	require.Equal(t, http.StatusAccepted, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "retry automatically")
	_, err = st.GetUser(ctx, member.ID)
	require.ErrorIs(t, err, store.ErrNotFound)
	pending, err := st.PendingHostedIdentityDeletions(ctx)
	require.NoError(t, err)
	require.Equal(t, []string{"clerk:member"}, pending)
	require.NoError(t, ro.drainHostedAccountDeletions(ctx))
	pending, err = st.PendingHostedIdentityDeletions(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, pending, "a failed retry stays durable")
	hosted.deleteErr = nil
	require.NoError(t, ro.drainHostedAccountDeletions(ctx))
	pending, err = st.PendingHostedIdentityDeletions(ctx)
	require.NoError(t, err)
	require.Empty(t, pending)
	require.Equal(t, []string{"clerk:member", "clerk:member", "clerk:member"}, hosted.deleted)
	_, err = st.UpsertHostedUser(ctx, "clerk:member", "member@example.test", time.Now().Add(time.Second).UnixMilli())
	require.ErrorIs(t, err, store.ErrNotFound)
}

// A preflight check is only a snapshot. If another admin disappears during
// revocation, the transactional guard must still run before Clerk user deletion.
func TestClerkDeleteDoesNotRemoveProviderUserWhenStoreGuardRefuses(t *testing.T) {
	ctx := context.Background()
	hosted := &deletionHostedLogin{createdAt: time.Now().Add(-time.Hour).UnixMilli()}
	ro, st := newLoginTestRouter(t, nil, nil, func(c *Config) { c.Clerk = hosted; c.AuthToken = "" })
	admin, err := st.UpsertHostedUser(ctx, "clerk:admin", "admin@example.test", hosted.createdAt)
	require.NoError(t, err)
	member, err := st.UpsertHostedUser(ctx, "clerk:member", "member@example.test", hosted.createdAt)
	require.NoError(t, err)
	require.NoError(t, st.SetHostedUserAdmin(ctx, member.ID, true))
	hosted.onRevoke = func() { require.NoError(t, st.SetHostedUserAdmin(ctx, admin.ID, false)) }
	r := httptest.NewRequest("DELETE", "/api/account", strings.NewReader(`{"confirm":"delete my library"}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Test-Clerk-Subject", "clerk:member")
	w := httptest.NewRecorder()
	ro.ServeHTTP(w, r)
	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	require.Empty(t, hosted.deleted)
	_, err = st.GetUser(ctx, member.ID)
	require.NoError(t, err)
	pending, err := st.PendingHostedIdentityDeletions(ctx)
	require.NoError(t, err)
	require.Empty(t, pending)
}
