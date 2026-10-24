package store

import "context"

// Hosted deployments only admit Clerk identities. Old local/OIDC rows remain
// in the database but cannot provide administrator succession in that mode.
// Keep the predicate in the write itself so concurrent demotions cannot both
// conclude that the other administrator will remain available.
const lastEnabledHostedAdminGuard = `(is_admin = 0 OR disabled_at IS NOT NULL
    OR substr(external_id, 1, 6) <> 'clerk:' OR EXISTS (
      SELECT 1 FROM users WHERE id <> ?1 AND is_admin = 1 AND disabled_at IS NULL
      AND substr(external_id, 1, 6) = 'clerk:' AND length(external_id) > 6))`

func (s *SQLiteStore) SetHostedUserAdmin(ctx context.Context, userID int64, admin bool) error {
	if admin {
		return s.exactlyOneRow(ctx,
			"UPDATE users SET is_admin = 1 WHERE id = ?1 AND substr(external_id, 1, 6) = 'clerk:' AND length(external_id) > 6", userID)
	}
	return s.guardedUserUpdate(ctx,
		"UPDATE users SET is_admin = 0 WHERE id = ?1 AND "+lastEnabledHostedAdminGuard, userID)
}

func (s *SQLiteStore) SetHostedUserDisabled(ctx context.Context, userID int64, disabled bool) error {
	if !disabled {
		return s.exactlyOneRow(ctx, "UPDATE users SET disabled_at = NULL WHERE id = ?1", userID)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx,
		`UPDATE users SET disabled_at = COALESCE(disabled_at, datetime('now'))
           WHERE id = ?1 AND `+lastEnabledHostedAdminGuard, userID)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return refusedOrMissing(ctx, tx, userID)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM sessions WHERE user_id = ?", userID); err != nil {
		return err
	}
	return tx.Commit()
}
