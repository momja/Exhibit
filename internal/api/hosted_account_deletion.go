package api

import (
	"context"
	"log/slog"
	"time"
)

func (ro *Router) deleteHostedIdentity(ctx context.Context, subject string) error {
	if err := ro.cfg.Clerk.DeleteUser(ctx, subject); err != nil {
		return err
	}
	return ro.cfg.Store.CompleteHostedIdentityDeletion(ctx, subject)
}

// RetryHostedAccountDeletions finishes durable provider work after failures or
// restarts. Only the local deletion transaction can enqueue an identity, after
// its last-administrator guard succeeds. Provider deletion is idempotent.
func (ro *Router) RetryHostedAccountDeletions(ctx context.Context) {
	if ro.cfg.Clerk == nil {
		return
	}
	timer := time.NewTicker(time.Minute)
	defer timer.Stop()
	for {
		if err := ro.drainHostedAccountDeletions(ctx); err != nil {
			slog.ErrorContext(ctx, "retry hosted account deletions", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
	}
}

func (ro *Router) drainHostedAccountDeletions(ctx context.Context) error {
	subjects, err := ro.cfg.Store.PendingHostedIdentityDeletions(ctx)
	if err != nil {
		return err
	}
	for _, subject := range subjects {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := ro.deleteHostedIdentity(ctx, subject); err != nil {
			slog.ErrorContext(ctx, "delete hosted identity", "error", err)
		}
	}
	return nil
}
