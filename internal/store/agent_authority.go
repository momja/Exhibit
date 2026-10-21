package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/momja/Exhibit/internal/agentcontrol"
)

var _ agentcontrol.Repository = (*SQLiteStore)(nil)

// Use a write transaction before reading admission/generation state. SQLite's
// single writer then serializes even independent repository connections. Setting
// busy_timeout on the borrowed connection bounds concurrent-core contention.
func (s *SQLiteStore) agentTransaction(ctx context.Context, fn func(*sql.Conn) error) error {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err = conn.ExecContext(ctx, `PRAGMA busy_timeout=5000`); err != nil {
		return err
	}
	if _, err = conn.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		return err
	}
	defer conn.ExecContext(context.Background(), `ROLLBACK`)
	if err = fn(conn); err != nil {
		return err
	}
	_, err = conn.ExecContext(ctx, `COMMIT`)
	return err
}

type agentQuery interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

const agentPreviousStatus = `COALESCE((SELECT p.status FROM agent_executions p WHERE p.conversation_id=e.conversation_id AND p.generation<e.generation ORDER BY p.generation DESC LIMIT 1),'')`
const agentAssignmentSelect = `SELECT c.owner_id,c.id,e.execution_id,e.worker_id,e.generation,c.artifact_id,e.status,e.lease_until,c.snapshot_revision,` + agentPreviousStatus + `
 FROM agent_executions e JOIN agent_conversations c ON c.id=e.conversation_id `
const liveAgentStatuses = `('reserved','active','closing')`

func scanAgentAssignment(row interface{ Scan(...any) error }) (agentcontrol.Assignment, error) {
	var a agentcontrol.Assignment
	var expiry int64
	err := row.Scan(&a.OwnerID, &a.ConversationID, &a.ExecutionID, &a.WorkerID, &a.Generation, &a.ArtifactID, &a.Status, &expiry, &a.Revision, &a.PreviousStatus)
	a.LeaseUntil = time.UnixMilli(expiry).UTC()
	if errors.Is(err, sql.ErrNoRows) {
		return a, agentcontrol.ErrStale
	}
	return a, err
}
func agentAssignment(ctx context.Context, q agentQuery, id agentcontrol.Identity, live bool, now time.Time) (agentcontrol.Assignment, error) {
	query := agentAssignmentSelect + ` WHERE c.owner_id=? AND c.id=? AND e.execution_id=? AND e.worker_id=? AND e.generation=?`
	args := []any{id.OwnerID, id.ConversationID, id.ExecutionID, id.WorkerID, id.Generation}
	if live {
		query += ` AND e.generation=c.generation AND e.status IN ` + liveAgentStatuses + ` AND e.lease_until>? AND (c.artifact_id='' OR EXISTS(SELECT 1 FROM artifacts a WHERE a.id=c.artifact_id AND a.owner_id=c.owner_id))`
		args = append(args, now.UnixMilli())
	}
	return scanAgentAssignment(q.QueryRowContext(ctx, query, args...))
}
func (s *SQLiteStore) GetAgentExecution(ctx context.Context, id agentcontrol.Identity, now time.Time) (agentcontrol.Assignment, error) {
	return agentAssignment(ctx, s.db, id, true, now)
}
func (s *SQLiteStore) ReserveAgentExecution(ctx context.Context, r agentcontrol.Reservation, now time.Time) (out agentcontrol.Assignment, err error) {
	if r.OwnerID <= 0 || r.ConversationID == "" || r.ExecutionID == "" || r.WorkerID == "" || r.FleetLimit <= 0 || !r.LeaseUntil.After(now) || len(r.ConversationID) > 200 || len(r.ExecutionID) > 200 || len(r.WorkerID) > 200 || len(r.ArtifactID) > 200 {
		return out, agentcontrol.ErrInvalid
	}
	err = s.agentTransaction(ctx, func(tx *sql.Conn) error {
		// Only this conversation is exclusively fenced by the caller. Leave other
		// expired/orphaned attempts for their own fenced reconciliation.
		_, err := tx.ExecContext(ctx, `UPDATE agent_executions SET status='interrupted' WHERE conversation_id=? AND status IN `+liveAgentStatuses+` AND (lease_until<=? OR conversation_id IN (SELECT id FROM agent_conversations c WHERE artifact_id!='' AND NOT EXISTS(SELECT 1 FROM artifacts a WHERE a.id=c.artifact_id AND a.owner_id=c.owner_id)))`, r.ConversationID, now.UnixMilli())
		if err != nil {
			return err
		}
		prior, e := scanAgentAssignment(tx.QueryRowContext(ctx, agentAssignmentSelect+` WHERE e.execution_id=?`, r.ExecutionID))
		if e == nil {
			if prior.OwnerID != r.OwnerID || prior.ConversationID != r.ConversationID || prior.WorkerID != r.WorkerID || (r.ArtifactID != "" && prior.ArtifactID != r.ArtifactID) {
				return agentcontrol.ErrConflict
			}
			if prior.Status != agentcontrol.Reserved && prior.Status != agentcontrol.Active && prior.Status != agentcontrol.Closing {
				return agentcontrol.ErrStale
			}
			out = prior
			return nil
		}
		if !errors.Is(e, agentcontrol.ErrStale) {
			return e
		}
		if r.ArtifactID != "" {
			var owned int
			if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM artifacts WHERE id=? AND owner_id=?)`, r.ArtifactID, r.OwnerID).Scan(&owned); err != nil {
				return err
			}
			if owned == 0 {
				return agentcontrol.ErrNotFound
			}
		}
		var owner, generation int64
		var artifact string
		e = tx.QueryRowContext(ctx, `SELECT owner_id,artifact_id,generation FROM agent_conversations WHERE id=?`, r.ConversationID).Scan(&owner, &artifact, &generation)
		if e != nil && !errors.Is(e, sql.ErrNoRows) {
			return e
		}
		exists := e == nil
		if exists && (owner != r.OwnerID || artifact != r.ArtifactID) {
			return agentcontrol.ErrNotFound
		}
		if r.Resume {
			if r.ArtifactID == "" {
				return agentcontrol.ErrInvalid
			}
			var resumable bool
			e = tx.QueryRowContext(ctx, `SELECT typeof(session_file)='text' FROM agent_transcripts WHERE artifact_id=? AND session_id=?`, r.ArtifactID, r.ConversationID).Scan(&resumable)
			if errors.Is(e, sql.ErrNoRows) {
				return agentcontrol.ErrNotFound
			}
			if e != nil {
				return e
			}
			if !resumable {
				return agentcontrol.ErrNotResumable
			}
		} else {
			var hasTranscript bool
			if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM agent_transcripts WHERE artifact_id=? AND session_id=?)`, r.ArtifactID, r.ConversationID).Scan(&hasTranscript); err != nil {
				return err
			}
			if hasTranscript {
				return agentcontrol.ErrAssigned
			}
		}
		var assigned int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM agent_executions WHERE conversation_id=? AND status IN `+liveAgentStatuses, r.ConversationID).Scan(&assigned); err != nil {
			return err
		}
		if assigned != 0 {
			return agentcontrol.ErrAssigned
		}
		var ownerCount, fleetCount int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(SUM(c.owner_id=?),0) FROM agent_executions e JOIN agent_conversations c ON c.id=e.conversation_id WHERE e.status IN `+liveAgentStatuses+` AND e.lease_until>? AND (c.artifact_id='' OR EXISTS(SELECT 1 FROM artifacts a WHERE a.id=c.artifact_id AND a.owner_id=c.owner_id))`, r.OwnerID, now.UnixMilli()).Scan(&fleetCount, &ownerCount); err != nil {
			return err
		}
		if ownerCount >= agentcontrol.MaxOwnerExecutions {
			return agentcontrol.ErrOwnerLimit
		}
		if fleetCount >= r.FleetLimit {
			return agentcontrol.ErrFleetLimit
		}
		if !exists {
			if _, err := tx.ExecContext(ctx, `INSERT INTO agent_conversations(id,owner_id,artifact_id) VALUES(?,?,?)`, r.ConversationID, r.OwnerID, r.ArtifactID); err != nil {
				return err
			}
		}
		generation++
		if _, err := tx.ExecContext(ctx, `UPDATE agent_conversations SET generation=? WHERE id=?`, generation, r.ConversationID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO agent_executions(execution_id,conversation_id,worker_id,generation,status,lease_until,artifact_version_seq) VALUES(?,?,?,?,'reserved',?,COALESCE((SELECT MAX(seq) FROM artifact_versions WHERE artifact_id=?),0))`, r.ExecutionID, r.ConversationID, r.WorkerID, generation, r.LeaseUntil.UnixMilli(), r.ArtifactID); err != nil {
			return err
		}
		out, err = agentAssignment(ctx, tx, agentcontrol.Identity{OwnerID: r.OwnerID, ConversationID: r.ConversationID, ExecutionID: r.ExecutionID, WorkerID: r.WorkerID, Generation: generation}, true, now)
		return err
	})
	return out, err
}
func (s *SQLiteStore) TransitionAgentExecution(ctx context.Context, id agentcontrol.Identity, status string, until, now time.Time) error {
	return s.agentTransaction(ctx, func(tx *sql.Conn) error {
		a, err := agentAssignment(ctx, tx, id, false, now)
		if err != nil {
			return err
		}
		terminal := status == agentcontrol.Closed || status == agentcontrol.Failed || status == agentcontrol.Interrupted
		if a.Status == agentcontrol.Closed || a.Status == agentcontrol.Failed || a.Status == agentcontrol.Interrupted {
			if a.Status == status {
				return nil
			}
			return agentcontrol.ErrStale
		}
		if !terminal {
			if _, err := agentAssignment(ctx, tx, id, true, now); err != nil {
				return err
			}
			if status != "" && status != agentcontrol.Active && status != agentcontrol.Closing {
				return agentcontrol.ErrInvalid
			}
			if a.Status == agentcontrol.Closing && status == agentcontrol.Active {
				return agentcontrol.ErrStale
			}
			if !until.After(now) {
				return agentcontrol.ErrInvalid
			}
		}
		if status == "" {
			status = a.Status
		}
		_, err = tx.ExecContext(ctx, `UPDATE agent_executions SET status=?,lease_until=MAX(lease_until,?) WHERE execution_id=?`, status, until.UnixMilli(), id.ExecutionID)
		return err
	})
}
func (s *SQLiteStore) BindAgentExecution(ctx context.Context, id agentcontrol.Identity, artifact string, now time.Time) error {
	if artifact == "" {
		return agentcontrol.ErrInvalid
	}
	return s.agentTransaction(ctx, func(tx *sql.Conn) error {
		a, err := agentAssignment(ctx, tx, id, true, now)
		if err != nil {
			return err
		}
		if a.ArtifactID != "" {
			if a.ArtifactID == artifact {
				return nil
			}
			return agentcontrol.ErrConflict
		}
		var owned int
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM artifacts WHERE id=? AND owner_id=?)`, artifact, id.OwnerID).Scan(&owned); err != nil {
			return err
		}
		if owned == 0 {
			return agentcontrol.ErrNotFound
		}
		_, err = tx.ExecContext(ctx, `UPDATE agent_conversations SET artifact_id=? WHERE id=? AND artifact_id=''`, artifact, id.ConversationID)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE agent_executions SET artifact_version_seq=COALESCE((SELECT MAX(seq) FROM artifact_versions WHERE artifact_id=?),0) WHERE execution_id=?`, artifact, id.ExecutionID)
		return err
	})
}

func (s *SQLiteStore) RecordAgentArtifactVersion(ctx context.Context, id agentcontrol.Identity, version int, now time.Time) error {
	if version <= 0 {
		return agentcontrol.ErrInvalid
	}
	return s.agentTransaction(ctx, func(tx *sql.Conn) error {
		a, err := agentAssignment(ctx, tx, id, true, now)
		if err != nil {
			return err
		}
		var valid bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM artifact_versions WHERE artifact_id=? AND seq=? AND session_id=? AND origin='agent')`, a.ArtifactID, version, id.ConversationID).Scan(&valid); err != nil {
			return err
		}
		if !valid {
			return agentcontrol.ErrConflict
		}
		_, err = tx.ExecContext(ctx, `UPDATE agent_executions SET artifact_version_seq=MAX(artifact_version_seq,?) WHERE execution_id=?`, version, id.ExecutionID)
		return err
	})
}

// Discovery takes no operation fences and changes no authority. Each candidate
// must be rechecked and reconciled under its own conversation's exclusive fence.
func (s *SQLiteStore) ListAgentExecutionsToReconcile(ctx context.Context, all bool, now time.Time) ([]agentcontrol.Assignment, error) {
	condition, args := agentReconciliationFilter(all, now)
	rows, err := s.db.QueryContext(ctx, agentAssignmentSelect+` WHERE `+condition+` ORDER BY c.id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []agentcontrol.Assignment{}
	for rows.Next() {
		a, err := scanAgentAssignment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func agentReconciliationFilter(all bool, now time.Time) (string, []any) {
	condition := `e.status IN ` + liveAgentStatuses
	if all {
		return condition, nil
	}
	return condition + ` AND (e.lease_until<=? OR (c.artifact_id!='' AND NOT EXISTS(SELECT 1 FROM artifacts a WHERE a.id=c.artifact_id AND a.owner_id=c.owner_id)))`, []any{now.UnixMilli()}
}

// ReconcileAgentExecution cannot interrupt a newer attempt, a terminal attempt,
// or a lease renewed after candidate discovery. A nil result means it no longer
// qualifies. The caller holds only this conversation's exclusive fence.
func (s *SQLiteStore) ReconcileAgentExecution(ctx context.Context, id agentcontrol.Identity, all bool, now time.Time) (out *agentcontrol.Assignment, err error) {
	err = s.agentTransaction(ctx, func(tx *sql.Conn) error {
		condition, args := agentReconciliationFilter(all, now)
		condition += ` AND c.owner_id=? AND c.id=? AND e.execution_id=? AND e.worker_id=? AND e.generation=?`
		args = append(args, id.OwnerID, id.ConversationID, id.ExecutionID, id.WorkerID, id.Generation)
		a, err := scanAgentAssignment(tx.QueryRowContext(ctx, agentAssignmentSelect+` WHERE `+condition, args...))
		if errors.Is(err, agentcontrol.ErrStale) {
			return nil
		}
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE agent_executions SET status='interrupted' WHERE execution_id=?`, id.ExecutionID); err != nil {
			return err
		}
		a.Status = agentcontrol.Interrupted
		out = &a
		return nil
	})
	return out, err
}
func (s *SQLiteStore) SaveAgentSupervisor(ctx context.Context, id agentcontrol.Identity, hash []byte, paidBy string, expires, now time.Time) error {
	if len(hash) != 32 || !expires.After(now) || (paidBy != PaidByPlatform && paidBy != PaidByUser) {
		return agentcontrol.ErrInvalid
	}
	return s.agentTransaction(ctx, func(tx *sql.Conn) error {
		if _, err := agentAssignment(ctx, tx, id, true, now); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO agent_supervisor_credentials(execution_id,token_hash,paid_by,expires_at) VALUES(?,?,?,?) ON CONFLICT(execution_id) DO UPDATE SET token_hash=excluded.token_hash,paid_by=excluded.paid_by,expires_at=excluded.expires_at`, id.ExecutionID, hash, paidBy, expires.UnixMilli())
		return err
	})
}
func (s *SQLiteStore) ResolveAgentSupervisor(ctx context.Context, hash []byte, now time.Time) (out agentcontrol.Supervisor, err error) {
	// Terminal attempts remain authenticatable for bounded late usage reporting;
	// snapshot endpoints separately require current, live assignment authority.
	var expiry int64
	err = s.db.QueryRowContext(ctx, `SELECT c.owner_id,c.id,e.execution_id,e.worker_id,e.generation,c.artifact_id,e.status,e.lease_until,c.snapshot_revision,k.paid_by,`+agentPreviousStatus+`
 FROM agent_supervisor_credentials k JOIN agent_executions e ON e.execution_id=k.execution_id JOIN agent_conversations c ON c.id=e.conversation_id
 WHERE k.token_hash=? AND k.expires_at>?`, hash, now.UnixMilli()).Scan(&out.OwnerID, &out.ConversationID, &out.ExecutionID, &out.WorkerID, &out.Generation, &out.ArtifactID, &out.Status, &expiry, &out.Revision, &out.PaidBy, &out.PreviousStatus)
	out.LeaseUntil = time.UnixMilli(expiry).UTC()
	if errors.Is(err, sql.ErrNoRows) {
		err = agentcontrol.ErrCredential
	}
	return out, err
}
func readAgentSnapshot(ctx context.Context, q agentQuery, a agentcontrol.Assignment, contents bool) (out agentcontrol.Snapshot, err error) {
	out.PreviousStatus = a.PreviousStatus
	var size sql.NullInt64
	// A legacy/imported row might predate the input bound. Check byte length
	// before asking the driver to allocate its entire string. Both reads are
	// inside the caller's transaction, so the checked file cannot change.
	err = q.QueryRowContext(ctx, `SELECT length(CAST(t.session_file AS BLOB)),t.title,t.version_seq,c.snapshot_revision,c.checkpoint_kind FROM agent_transcripts t JOIN agent_conversations c ON c.artifact_id=t.artifact_id AND c.id=t.session_id WHERE c.id=? AND c.owner_id=?`, a.ConversationID, a.OwnerID).Scan(&size, &out.Title, &out.VersionSeq, &out.Revision, &out.Kind)
	if errors.Is(err, sql.ErrNoRows) {
		return out, agentcontrol.ErrNotFound
	}
	if err != nil {
		return out, err
	}
	if !size.Valid {
		return out, agentcontrol.ErrNotResumable
	}
	if size.Int64 > agentcontrol.MaxSnapshotBytes {
		return out, agentcontrol.ErrTooLarge
	}
	if contents {
		err = q.QueryRowContext(ctx, `SELECT session_file FROM agent_transcripts WHERE artifact_id=? AND session_id=?`, a.ArtifactID, a.ConversationID).Scan(&out.SessionFile)
	}
	return out, err
}
func (s *SQLiteStore) ReadAgentSnapshot(ctx context.Context, id agentcontrol.Identity, now time.Time) (out agentcontrol.Snapshot, err error) {
	err = s.agentTransaction(ctx, func(tx *sql.Conn) error {
		a, err := agentAssignment(ctx, tx, id, true, now)
		if err != nil {
			return err
		}
		out, err = readAgentSnapshot(ctx, tx, a, true)
		return err
	})
	return out, err
}
func (s *SQLiteStore) WriteAgentCheckpoint(ctx context.Context, id agentcontrol.Identity, p agentcontrol.Checkpoint, now time.Time) (out agentcontrol.Snapshot, err error) {
	if err = agentcontrol.ValidateCheckpoint(p); err != nil {
		return out, err
	}
	err = s.agentTransaction(ctx, func(tx *sql.Conn) error {
		a, err := agentAssignment(ctx, tx, id, true, now)
		if err != nil {
			return err
		}
		if a.ArtifactID == "" {
			return agentcontrol.ErrNotFound
		}
		var revision int64
		var hash string
		if err := tx.QueryRowContext(ctx, `SELECT snapshot_revision,checkpoint_hash FROM agent_conversations WHERE id=?`, id.ConversationID).Scan(&revision, &hash); err != nil {
			return err
		}
		digest := agentcontrol.CheckpointHash(p)
		if p.Revision < revision {
			return agentcontrol.ErrConflict
		}
		if p.Revision == revision {
			if digest != hash {
				return agentcontrol.ErrConflict
			}
			out, err = readAgentSnapshot(ctx, tx, a, false)
			return err
		}
		var title string
		err = tx.QueryRowContext(ctx, `SELECT title FROM agent_transcripts WHERE artifact_id=? AND session_id=?`, a.ArtifactID, id.ConversationID).Scan(&title)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if title != "" && p.Title != title {
			return agentcontrol.ErrConflict
		}
		// Use the attempt's core-recorded version, never an upload-time head that
		// may have moved on in a browser, nor a worker-supplied version number.
		_, err = tx.ExecContext(ctx, `INSERT INTO agent_transcripts(artifact_id,session_id,title,session_file,version_seq,updated_at) VALUES(?,?,?,?,(SELECT artifact_version_seq FROM agent_executions WHERE execution_id=?),datetime('now')) ON CONFLICT(artifact_id,session_id) DO UPDATE SET title=excluded.title,session_file=excluded.session_file,version_seq=excluded.version_seq,updated_at=excluded.updated_at`, a.ArtifactID, id.ConversationID, p.Title, p.SessionFile, id.ExecutionID)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE agent_conversations SET snapshot_revision=?,checkpoint_hash=?,checkpoint_kind=? WHERE id=?`, p.Revision, digest, p.Kind, id.ConversationID); err != nil {
			return err
		}
		out, err = readAgentSnapshot(ctx, tx, a, false)
		return err
	})
	return out, err
}
func (s *SQLiteStore) ReportAgentUsage(ctx context.Context, id agentcontrol.Identity, paidBy string, u agentcontrol.Usage) (inserted bool, err error) {
	if err = agentcontrol.ValidateUsage(u); err != nil {
		return false, err
	}
	err = s.agentTransaction(ctx, func(tx *sql.Conn) error {
		if _, err := agentAssignment(ctx, tx, id, false, time.Time{}); err != nil {
			return err
		}
		// Funding attribution comes from the core's credential record even for late
		// reports. Guardrail is always operator spend, including BYOK attempts.
		var assignedPaidBy string
		if err := tx.QueryRowContext(ctx, `SELECT paid_by FROM agent_supervisor_credentials WHERE execution_id=?`, id.ExecutionID).Scan(&assignedPaidBy); err != nil {
			return err
		}
		if paidBy != assignedPaidBy {
			return agentcontrol.ErrCredential
		}
		if u.Source == UsageSourceGuardrail {
			paidBy = PaidByPlatform
		}
		digest := agentcontrol.UsageHash(u)
		var prior string
		err := tx.QueryRowContext(ctx, `SELECT payload_hash FROM agent_usage_receipts WHERE execution_id=? AND event_id=?`, id.ExecutionID, u.EventID).Scan(&prior)
		if err == nil {
			if prior != digest {
				return agentcontrol.ErrConflict
			}
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO agent_usage_receipts(execution_id,event_id,payload_hash) VALUES(?,?,?)`, id.ExecutionID, u.EventID, digest); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO agent_usage(owner_id,session_id,provider,model,paid_by,source,input_tokens,output_tokens,cache_read_tokens,cache_write_tokens,cost_micros) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, id.OwnerID, id.ConversationID, u.Provider, u.Model, paidBy, u.Source, u.InputTokens, u.OutputTokens, u.CacheReadTokens, u.CacheWriteTokens, u.CostMicros); err != nil {
			return err
		}
		inserted = true
		return nil
	})
	return inserted, err
}

// AgentArtifactWriter is the narrow extension used by the core's tool create
// path. Store remains unchanged. The caller holds Grant.Guard through staging,
// this transaction and BindArtifactContext, and supplies the core's clock.
// Using ordinary PutArtifact followed by Bind is insufficient: a failed bind
// would leave a committed artifact and an unbound grant able to create again.
type AgentArtifactWriter interface {
	PutAgentArtifact(context.Context, agentcontrol.Identity, *Artifact, time.Time) error
}

// PutAgentArtifact commits the initial artifact/version and one-time durable
// binding together. Owner and conversation provenance come from verified
// authority, never from fields on the input artifact.
func (s *SQLiteStore) PutAgentArtifact(ctx context.Context, id agentcontrol.Identity, input *Artifact, now time.Time) error {
	if input == nil || input.ID == "" {
		return agentcontrol.ErrInvalid
	}
	return s.agentTransaction(ctx, func(tx *sql.Conn) error {
		assignment, err := agentAssignment(ctx, tx, id, true, now)
		if err != nil {
			return err
		}
		if assignment.ArtifactID != "" {
			return agentcontrol.ErrConflict
		}
		a := *input
		a.OwnerID = id.OwnerID
		a.Provenance.Origin = VersionAgent
		a.Provenance.SessionID = id.ConversationID
		if err := putArtifactTx(ctx, tx, &a); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE agent_conversations SET artifact_id=? WHERE id=?`, a.ID, id.ConversationID); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE agent_executions SET artifact_version_seq=1 WHERE execution_id=?`, id.ExecutionID)
		return err
	})
}
