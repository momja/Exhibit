package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// UpsertHostedUser resolves a verified Clerk session. The creation timestamp is
// the provider session's immutable created_at, not a refreshed JWT's iat. Read
// the erasure fence and resolve the user in one transaction: an in-flight
// request must not recreate a library deleted after its authentication began.
// ErrNotFound means the session predates erasure and a fresh login is required.
func (s *SQLiteStore) UpsertHostedUser(ctx context.Context, externalID, email string, sessionCreatedAt int64) (*User, error) {
	if !strings.HasPrefix(externalID, "clerk:") || len(externalID) == len("clerk:") || sessionCreatedAt <= 0 {
		return nil, errors.New("hosted login requires a Clerk identity and session creation time")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var cutoff int64
	err = tx.QueryRowContext(ctx, "SELECT deleted_at_ms FROM hosted_account_deletions WHERE external_id=?", externalID).Scan(&cutoff)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if err == nil && sessionCreatedAt <= cutoff {
		return nil, ErrNotFound
	}

	var sc userScan
	err = tx.QueryRowContext(ctx, "SELECT "+userColumns+" FROM users WHERE external_id=?", externalID).Scan(sc.dest()...)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = tx.ExecContext(ctx, `INSERT INTO users (external_id, email, is_admin)
			SELECT ?, ?, NOT EXISTS (SELECT 1 FROM users)`, externalID, email)
		if err != nil {
			return nil, err
		}
		err = tx.QueryRowContext(ctx, "SELECT "+userColumns+" FROM users WHERE external_id=?", externalID).Scan(sc.dest()...)
	}
	if err != nil {
		return nil, err
	}
	u := sc.user()
	if u.Email != email {
		if _, err := tx.ExecContext(ctx, "UPDATE users SET email=? WHERE id=?", email, u.ID); err != nil {
			return nil, err
		}
		u.Email = email
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return u, nil
}
