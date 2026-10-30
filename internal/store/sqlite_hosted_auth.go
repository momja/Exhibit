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
// ErrNotFound means the identity was deleted, or a legacy library-only erasure
// requires a fresh session. New account deletion permanently fences the identity.
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
	var deleteIdentity bool
	err = tx.QueryRowContext(ctx, "SELECT deleted_at_ms, delete_identity FROM hosted_account_deletions WHERE external_id=?", externalID).Scan(&cutoff, &deleteIdentity)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if err == nil && (deleteIdentity || sessionCreatedAt <= cutoff) {
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

// PendingHostedIdentityDeletions survives local account erasure and restarts.
func (s *SQLiteStore) PendingHostedIdentityDeletions(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT external_id FROM hosted_account_deletions WHERE delete_identity=1 AND identity_deleted=0 ORDER BY deleted_at_ms`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var subjects []string
	for rows.Next() {
		var subject string
		if err := rows.Scan(&subject); err != nil {
			return nil, err
		}
		subjects = append(subjects, subject)
	}
	return subjects, rows.Err()
}

func (s *SQLiteStore) CompleteHostedIdentityDeletion(ctx context.Context, externalID string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE hosted_account_deletions SET identity_deleted=1 WHERE external_id=? AND delete_identity=1`, externalID)
	return err
}
