package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Relay grants are viewer capabilities. An origin decision alone never enables
// server requests, and sharing an artifact never shares a viewer's consent.
// There is one decision, not a read/write pair: an HTTP method says nothing
// reliable about what a request does, so splitting on it would promise a
// boundary the relay cannot enforce (Exh-cxzb).
type RelayGrant struct {
	Allowed bool `json:"allowed"`
}

type RelayActivity struct {
	Origin     string    `json:"origin"`
	Method     string    `json:"method"`
	Status     int       `json:"status"`
	Bytes      int       `json:"bytes"`
	DurationMS int64     `json:"duration_ms"`
	Cached     bool      `json:"cached"`
	ErrorCode  string    `json:"error_code,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// Schedules contain data, never executable artifact code. Only successful
// responses are written to state; a failed refresh retains the previous value.
type RelaySchedule struct {
	ID              string `json:"id"`
	ArtifactID      string `json:"artifact_id"`
	ViewerID        int64  `json:"-"`
	URL             string `json:"url"`
	StateKey        string `json:"state_key"`
	IntervalSeconds int64  `json:"interval_seconds"`
	NextRunAt       int64  `json:"next_run_at"`
	LastRunAt       int64  `json:"last_run_at"`
	LastStatus      int    `json:"last_status"`
	LastError       string `json:"last_error"`
	Enabled         bool   `json:"enabled"`
}

// RelayStore is separate from Store so older adapters can keep serving the
// non-experimental product. Enabling the relay requires this entire interface.
type RelayStore interface {
	GetRelayGrant(context.Context, int64, string) (RelayGrant, error)
	SetRelayGrant(context.Context, int64, string, RelayGrant) error
	RecordRelayActivity(context.Context, int64, string, RelayActivity) error
	ListRelayActivity(context.Context, int64, string) ([]RelayActivity, error)
	PutRelaySchedule(context.Context, RelaySchedule, int) error
	DeleteRelaySchedule(context.Context, int64, string, string) error
	ListRelaySchedules(context.Context, int64, string) ([]RelaySchedule, error)
	CountRelaySchedules(context.Context, int64) (int, error)
	ClaimRelaySchedules(context.Context, time.Time, int) ([]RelaySchedule, error)
	FinishRelaySchedule(context.Context, RelaySchedule, int, string) error
}

func (s *SQLiteStore) GetRelayGrant(ctx context.Context, viewer int64, artifact string) (RelayGrant, error) {
	var g RelayGrant
	err := s.db.QueryRowContext(ctx, `SELECT allowed FROM artifact_relay_grants WHERE artifact_id=? AND viewer_id=?`, artifact, viewer).Scan(&g.Allowed)
	if errors.Is(err, sql.ErrNoRows) {
		return g, nil
	}
	return g, err
}

func (s *SQLiteStore) SetRelayGrant(ctx context.Context, viewer int64, artifact string, g RelayGrant) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO artifact_relay_grants(artifact_id,viewer_id,allowed) VALUES (?,?,?) ON CONFLICT(artifact_id,viewer_id) DO UPDATE SET allowed=excluded.allowed,updated_at=CURRENT_TIMESTAMP`, artifact, viewer, g.Allowed)
	return err
}

func (s *SQLiteStore) RecordRelayActivity(ctx context.Context, viewer int64, artifact string, a RelayActivity) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO relay_activity(artifact_id,viewer_id,origin,method,status,bytes,duration_ms,cached,error_code) VALUES(?,?,?,?,?,?,?,?,?)`, artifact, viewer, a.Origin, a.Method, a.Status, a.Bytes, a.DurationMS, a.Cached, a.ErrorCode)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `DELETE FROM relay_activity WHERE artifact_id=? AND viewer_id=? AND id NOT IN (SELECT id FROM relay_activity WHERE artifact_id=? AND viewer_id=? ORDER BY id DESC LIMIT 100)`, artifact, viewer, artifact, viewer)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLiteStore) ListRelayActivity(ctx context.Context, viewer int64, artifact string) ([]RelayActivity, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT origin,method,status,bytes,duration_ms,cached,error_code,created_at FROM relay_activity WHERE artifact_id=? AND viewer_id=? ORDER BY id DESC LIMIT 30`, artifact, viewer)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RelayActivity{}
	for rows.Next() {
		var a RelayActivity
		var created any
		if err := rows.Scan(&a.Origin, &a.Method, &a.Status, &a.Bytes, &a.DurationMS, &a.Cached, &a.ErrorCode, &created); err != nil {
			return nil, err
		}
		a.CreatedAt = anyToTime(created)
		out = append(out, a)
	}
	return out, rows.Err()
}

// The limit check and insert share one transaction, including uncertain retries
// of an existing id. A viewer cannot race two creations past their allowance.
func (s *SQLiteStore) PutRelaySchedule(ctx context.Context, a RelaySchedule, limit int) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var count int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM relay_schedules WHERE viewer_id=? AND id != ?`, a.ViewerID, a.ID).Scan(&count); err != nil {
		return err
	}
	if limit >= 0 && count >= limit {
		return fmt.Errorf("schedule limit reached (%d)", limit)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO relay_schedules(id,artifact_id,viewer_id,url,state_key,interval_seconds,next_run_at,enabled) VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET url=excluded.url,state_key=excluded.state_key,interval_seconds=excluded.interval_seconds,next_run_at=excluded.next_run_at,enabled=excluded.enabled WHERE viewer_id=excluded.viewer_id AND artifact_id=excluded.artifact_id`, a.ID, a.ArtifactID, a.ViewerID, a.URL, a.StateKey, a.IntervalSeconds, a.NextRunAt, a.Enabled)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLiteStore) DeleteRelaySchedule(ctx context.Context, viewer int64, artifact, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM relay_schedules WHERE id=? AND artifact_id=? AND viewer_id=?`, id, artifact, viewer)
	return err
}

const scheduleColumns = `id,artifact_id,viewer_id,url,state_key,interval_seconds,next_run_at,last_run_at,last_status,last_error,enabled`

func readRelaySchedules(rows *sql.Rows) ([]RelaySchedule, error) {
	out := []RelaySchedule{}
	for rows.Next() {
		var a RelaySchedule
		if err := rows.Scan(&a.ID, &a.ArtifactID, &a.ViewerID, &a.URL, &a.StateKey, &a.IntervalSeconds, &a.NextRunAt, &a.LastRunAt, &a.LastStatus, &a.LastError, &a.Enabled); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) ListRelaySchedules(ctx context.Context, viewer int64, artifact string) ([]RelaySchedule, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+scheduleColumns+` FROM relay_schedules WHERE viewer_id=? AND artifact_id=? ORDER BY next_run_at`, viewer, artifact)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return readRelaySchedules(rows)
}

func (s *SQLiteStore) CountRelaySchedules(ctx context.Context, viewer int64) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM relay_schedules WHERE viewer_id=?`, viewer).Scan(&count)
	return count, err
}

// Claim advances next_run_at before returning work. This process has one
// scheduler, but this also prevents overlapping ticks and restart bursts.
func (s *SQLiteStore) ClaimRelaySchedules(ctx context.Context, now time.Time, limit int) ([]RelaySchedule, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT `+scheduleColumns+` FROM relay_schedules WHERE enabled=1 AND next_run_at<=? ORDER BY next_run_at LIMIT ?`, now.Unix(), limit)
	if err != nil {
		return nil, err
	}
	out, err := readRelaySchedules(rows)
	rows.Close()
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].NextRunAt = now.Unix() + out[i].IntervalSeconds
		_, err = tx.ExecContext(ctx, `UPDATE relay_schedules SET next_run_at=? WHERE id=?`, out[i].NextRunAt, out[i].ID)
		if err != nil {
			return nil, err
		}
	}
	return out, tx.Commit()
}

func (s *SQLiteStore) FinishRelaySchedule(ctx context.Context, a RelaySchedule, status int, reason string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE relay_schedules SET last_run_at=?,last_status=?,last_error=? WHERE id=? AND viewer_id=?`, time.Now().Unix(), status, reason, a.ID, a.ViewerID)
	return err
}
