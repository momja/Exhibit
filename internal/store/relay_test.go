package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRelayScheduleClaimsAndAccountDeletion(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	fx := seedEverything(t, s)
	// Personal rows on another person's artifact must also be deleted. The
	// artifact cascade cannot reach these; the user-delete trigger must.
	require.NoError(t, s.PutArtifact(ctx, &Artifact{ID: "another-library", OwnerID: fx.admin.ID, Title: "Shared tool", Tier: Tier1}))
	require.NoError(t, s.SetRelayGrant(ctx, fx.member.ID, "another-library", RelayGrant{Allowed: true}))
	require.NoError(t, s.RecordRelayActivity(ctx, fx.member.ID, "another-library", RelayActivity{Origin: "https://api.example.test", Method: "GET", Status: 200}))
	now := time.Now()
	job := RelaySchedule{ID: "recipient-job", ViewerID: fx.member.ID, ArtifactID: "another-library", URL: "https://api.example.test", StateKey: "data", IntervalSeconds: 3600, NextRunAt: now.Unix(), Enabled: true}
	require.NoError(t, s.PutRelaySchedule(ctx, job, 10))
	jobs, err := s.ClaimRelaySchedules(ctx, now, 10)
	require.NoError(t, err)
	require.Len(t, jobs, 2)
	jobs, err = s.ClaimRelaySchedules(ctx, now, 10)
	require.NoError(t, err)
	require.Empty(t, jobs, "an overlapping scheduler tick must not repeat provider I/O")
	job.ID = "too-many"
	require.Error(t, s.PutRelaySchedule(ctx, job, 2))
	_, err = s.DeleteAccount(ctx, fx.member.ID)
	require.NoError(t, err)
	for _, table := range []string{"artifact_relay_grants", "relay_activity", "relay_schedules"} {
		var count int
		require.NoError(t, s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table+" WHERE viewer_id=?", fx.member.ID).Scan(&count))
		require.Zero(t, count, table)
	}
}
