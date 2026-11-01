package store

import (
	"context"
	"errors"
	"time"
)

// PendingVendorConnection holds revocation work, not usable credentials. Its
// request is pinned provider/owner metadata; it never includes an OAuth token.
type PendingVendorConnection struct {
	ID          string
	OwnerID     int64
	RequestJSON string
	ExpiresAt   time.Time
	CleanupAt   time.Time
}

func (s *SQLiteStore) CreatePendingVendorConnection(ctx context.Context, pending PendingVendorConnection) error {
	if pending.CleanupAt.IsZero() {
		pending.CleanupAt = pending.ExpiresAt
	}
	result, err := s.db.ExecContext(ctx, `INSERT INTO pending_vendor_connections(id,owner_id,request_json,expires_at,cleanup_at)
 SELECT ?,?,?,?,? WHERE (SELECT COUNT(*) FROM pending_vendor_connections)<512`, pending.ID, pending.OwnerID, pending.RequestJSON, pending.ExpiresAt.Unix(), pending.CleanupAt.Unix())
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n != 1 {
		return errors.New("too many pending managed connections; try again later")
	}
	return err
}

func (s *SQLiteStore) UpdatePendingVendorConnection(ctx context.Context, pending PendingVendorConnection) error {
	if pending.CleanupAt.IsZero() {
		pending.CleanupAt = pending.ExpiresAt
	}
	result, err := s.db.ExecContext(ctx, `UPDATE pending_vendor_connections SET request_json=?,expires_at=?,cleanup_at=? WHERE id=? AND owner_id=?`, pending.RequestJSON, pending.ExpiresAt.Unix(), pending.CleanupAt.Unix(), pending.ID, pending.OwnerID)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n != 1 {
		return ErrNotFound
	}
	return err
}

// A zero owner selects all owners; a zero cutoff includes unexpired flows for
// explicit account erasure. Periodic maintenance selects only expired flows.
func (s *SQLiteStore) ListPendingVendorConnections(ctx context.Context, ownerID int64, before time.Time) ([]PendingVendorConnection, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,owner_id,request_json,expires_at,cleanup_at FROM pending_vendor_connections
 WHERE (?=0 OR owner_id=?) AND (? OR cleanup_at<=?) ORDER BY cleanup_at,id`, ownerID, ownerID, before.IsZero(), before.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PendingVendorConnection
	for rows.Next() {
		var pending PendingVendorConnection
		var expires, cleanup int64
		if err := rows.Scan(&pending.ID, &pending.OwnerID, &pending.RequestJSON, &expires, &cleanup); err != nil {
			return nil, err
		}
		pending.ExpiresAt = time.Unix(expires, 0)
		pending.CleanupAt = time.Unix(cleanup, 0)
		out = append(out, pending)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) DeletePendingVendorConnection(ctx context.Context, ownerID int64, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM pending_vendor_connections WHERE id=? AND owner_id=?`, id, ownerID)
	return err
}

// Successful adoption may precede a crash or a cancelled response. Cleanup
// must recognize that durable record before revoking the remote account.
func (s *SQLiteStore) HasVendorConnectionReference(ctx context.Context, ownerID int64, engine, provider, externalRef string) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM connections WHERE owner_id=? AND kind='vendor' AND engine=? AND provider=? AND external_ref=?)`, ownerID, engine, provider, externalRef).Scan(&exists)
	return exists, err
}
