package store

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHostedAdminSuccessionIgnoresOtherIdentityProviders(t *testing.T) {
	for _, change := range []string{"demote", "disable"} {
		t.Run(change, func(t *testing.T) {
			s := newTestStore(t)
			ctx := context.Background()
			admin, err := s.UpsertUser(ctx, "clerk:admin", "admin@example.test")
			require.NoError(t, err)
			for _, subject := range []string{"local:legacy", "oidc:legacy", "Clerk:wrongcase", "clerk:"} {
				other, err := s.UpsertUser(ctx, subject, subject)
				require.NoError(t, err)
				require.NoError(t, s.SetUserAdmin(ctx, other.ID, true))
			}
			if change == "demote" {
				err = s.SetHostedUserAdmin(ctx, admin.ID, false)
			} else {
				err = s.SetHostedUserDisabled(ctx, admin.ID, true)
			}
			require.ErrorIs(t, err, ErrLastAdmin)
			other, err := s.UpsertUser(ctx, "clerk:successor", "successor@example.test")
			require.NoError(t, err)
			require.NoError(t, s.SetHostedUserAdmin(ctx, other.ID, true))
			if change == "demote" {
				err = s.SetHostedUserAdmin(ctx, admin.ID, false)
			} else {
				err = s.SetHostedUserDisabled(ctx, admin.ID, true)
			}
			require.NoError(t, err)
		})
	}
}

func TestHostedAdminChangesCannotRaceToRemoveAllAdmins(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	one, err := s.UpsertUser(ctx, "clerk:one", "one@example.test")
	require.NoError(t, err)
	two, err := s.UpsertUser(ctx, "clerk:two", "two@example.test")
	require.NoError(t, err)
	require.NoError(t, s.SetHostedUserAdmin(ctx, two.ID, true))
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); <-start; results <- s.SetHostedUserAdmin(ctx, one.ID, false) }()
	go func() { defer wg.Done(); <-start; results <- s.SetHostedUserDisabled(ctx, two.ID, true) }()
	close(start)
	wg.Wait()
	close(results)
	refused := 0
	for err := range results {
		if err != nil {
			require.ErrorIs(t, err, ErrLastAdmin)
			refused++
		}
	}
	require.Equal(t, 1, refused)
}
