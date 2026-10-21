package agent

import (
	"context"
	"testing"
	"time"

	"github.com/momja/Exhibit/internal/agentscope"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func capacityManager(t *testing.T, ceiling int) *Manager {
	t.Helper()
	m, err := New(Config{Credentials: agentscope.NewRegistry(), MaxSessions: ceiling}, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		require.NoError(t, m.Shutdown(ctx))
	})
	return m
}

func TestConfiguredInstanceCapacityIncludesRetainedDrains(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		configured, expected int
	}{
		{"default", 0, 32}, {"smaller", 2, 2}, {"larger-fleet", 40, 40},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := capacityManager(t, tc.configured)
			assert.Equal(t, tc.expected, m.cfg.MaxSessions)
			// A retained execution consumes capacity even after its OS process exits.
			m.recoveries["retained"] = &Session{OwnerID: 7}
			releases := []func(){}
			defer func() {
				for _, release := range releases {
					release()
				}
				delete(m.recoveries, "retained")
			}()
			for i := 0; i < tc.expected-1; i++ {
				release, err := m.reserveProcess(context.Background(), int64(100+i))
				require.NoError(t, err, "cross-owner fleet slot %d", i+1)
				releases = append(releases, release)
			}
			_, err := m.reserveProcess(context.Background(), 999)
			require.ErrorIs(t, err, ErrInstanceSessionLimit)
			delete(m.recoveries, "retained")
			release, err := m.reserveProcess(context.Background(), 999)
			require.NoError(t, err, "reclaiming the recovery returns exactly one slot")
			releases = append(releases, release)
		})
	}
}

func TestConfiguredFleetKeepsTenSessionOwnerCeiling(t *testing.T) {
	m := capacityManager(t, 48)
	m.recoveries["retained"] = &Session{OwnerID: 7}
	releases := []func(){}
	defer func() {
		for _, release := range releases {
			release()
		}
		delete(m.recoveries, "retained")
	}()
	for i := 0; i < MaxOwnerSessions-1; i++ {
		release, err := m.reserveProcess(context.Background(), 7)
		require.NoError(t, err)
		releases = append(releases, release)
	}
	_, err := m.reserveProcess(context.Background(), 7)
	require.ErrorIs(t, err, ErrSessionLimit)
	// Another owner still has fleet capacity.
	release, err := m.reserveProcess(context.Background(), 8)
	require.NoError(t, err)
	releases = append(releases, release)
	delete(m.recoveries, "retained")
}

func TestNegativeInstanceCapacityRejected(t *testing.T) {
	_, err := New(Config{Credentials: agentscope.NewRegistry(), MaxSessions: -1}, nil)
	require.ErrorContains(t, err, "maximum sessions")
}
