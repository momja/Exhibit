package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/momja/Exhibit/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelayScheduledRefreshChecksAccountAccess(t *testing.T) {
	for _, test := range []struct {
		name         string
		account      bool
		disabled     bool
		loginEnabled bool
		lookupFails  bool
		allowed      bool
	}{
		{name: "single-user default owner without a user row", allowed: true},
		{name: "enabled account", account: true, allowed: true},
		{name: "disabled account with limits off", account: true, disabled: true},
		{name: "missing account with login enabled", loginEnabled: true},
		{name: "account lookup fails", lookupFails: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int64
			ro, id, upstream := relayFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				fmt.Fprint(w, `{"balance":42}`)
			}))
			ctx := t.Context()
			if test.account {
				createScheduledAccount(t, ro)
			}
			if test.disabled {
				require.NoError(t, ro.cfg.Store.SetUserDisabled(ctx, 1, true))
			}
			ro.cfg.LocalUsers = test.loginEnabled
			s, ok := ro.relayStore()
			require.True(t, ok)
			require.NoError(t, s.SetRelayGrant(ctx, 1, id, store.RelayGrant{Allowed: true}))
			require.NoError(t, ro.cfg.Store.SetStateAsViewer(ctx, 1, id, "accounts", "previous data"))
			now := time.Now()
			job := store.RelaySchedule{ID: "refresh", ArtifactID: id, ViewerID: 1, URL: upstream.URL, StateKey: "accounts", IntervalSeconds: 3600, NextRunAt: now.Unix(), Enabled: true}
			require.NoError(t, s.PutRelaySchedule(ctx, job, 10))
			if test.lookupFails {
				ro.cfg.Store = &failedScheduledAccountStore{Store: ro.cfg.Store, RelayStore: s}
			}

			ro.runRelaySchedules(ctx, now)
			state, err := ro.cfg.Store.GetStateAsViewer(ctx, 1, id)
			require.NoError(t, err)
			jobs, err := s.ListRelaySchedules(ctx, 1, id)
			require.NoError(t, err)
			require.Len(t, jobs, 1)
			if test.allowed {
				assert.EqualValues(t, 1, calls.Load())
				assert.Equal(t, `{"balance":42}`, state["accounts"])
				assert.Empty(t, jobs[0].LastError)
			} else {
				assert.Zero(t, calls.Load(), "an unavailable account must not start provider I/O")
				assert.Equal(t, "previous data", state["accounts"])
				assert.Contains(t, jobs[0].LastError, "Account access")
			}
		})
	}
}

func TestRelayScheduledRefreshDiscardsResponseWhenAccountDisabledInFlight(t *testing.T) {
	var disable func() error
	disabled := make(chan error, 1)
	ro, id, upstream := relayFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		disabled <- disable()
		fmt.Fprint(w, "new data")
	}))
	ctx := t.Context()
	createScheduledAccount(t, ro)
	disable = func() error { return ro.cfg.Store.SetUserDisabled(ctx, 1, true) }
	s, ok := ro.relayStore()
	require.True(t, ok)
	require.NoError(t, s.SetRelayGrant(ctx, 1, id, store.RelayGrant{Allowed: true}))
	require.NoError(t, ro.cfg.Store.SetStateAsViewer(ctx, 1, id, "accounts", "previous data"))
	now := time.Now()
	job := store.RelaySchedule{ID: "refresh", ArtifactID: id, ViewerID: 1, URL: upstream.URL, StateKey: "accounts", IntervalSeconds: 3600, NextRunAt: now.Unix(), Enabled: true}
	require.NoError(t, s.PutRelaySchedule(ctx, job, 10))

	ro.runRelaySchedules(ctx, now)
	select {
	case err := <-disabled:
		require.NoError(t, err)
	default:
		t.Fatal("the refresh did not reach the provider")
	}
	state, err := ro.cfg.Store.GetStateAsViewer(ctx, 1, id)
	require.NoError(t, err)
	assert.Equal(t, "previous data", state["accounts"])
	jobs, err := s.ListRelaySchedules(ctx, 1, id)
	require.NoError(t, err)
	require.Len(t, jobs, 1)
	assert.Contains(t, jobs[0].LastError, "Account access changed")
}

func createScheduledAccount(t *testing.T, ro *Router) {
	t.Helper()
	ctx := t.Context()
	user, err := ro.cfg.Store.UpsertUser(ctx, "scheduled-user", "scheduled@example.test")
	require.NoError(t, err)
	require.EqualValues(t, 1, user.ID)
	// The first account is an admin; retain another enabled admin so disabling
	// the scheduled account is a valid administrative operation.
	admin, err := ro.cfg.Store.UpsertUser(ctx, "another-admin", "admin@example.test")
	require.NoError(t, err)
	require.NoError(t, ro.cfg.Store.SetUserAdmin(ctx, admin.ID, true))
}

type failedScheduledAccountStore struct {
	store.Store
	store.RelayStore
}

func (*failedScheduledAccountStore) GetUser(context.Context, int64) (*store.User, error) {
	return nil, errors.New("account storage unavailable")
}
