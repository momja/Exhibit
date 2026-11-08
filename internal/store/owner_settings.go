package store

import (
	"context"
	"database/sql"
	"errors"
)

// AgentStateAccessDefault reads the owner's account-wide answer to whether
// agents may read and change saved data (Exh-dabd). An owner who never set it
// has no row, and that reads as false: saved data stays out of an inference
// provider's context until the owner says otherwise.
func (s *SQLiteStore) AgentStateAccessDefault(ctx context.Context, ownerID int64) (bool, error) {
	var allowed bool
	err := s.db.QueryRowContext(ctx,
		"SELECT agent_state_access FROM owner_settings WHERE owner_id = ?", ownerID).Scan(&allowed)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return allowed, err
}

// SetAgentStateAccessDefault records the owner's account-wide answer.
func (s *SQLiteStore) SetAgentStateAccessDefault(ctx context.Context, ownerID int64, allowed bool) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO owner_settings (owner_id, agent_state_access, updated_at)
		VALUES (?, ?, datetime('now'))
		ON CONFLICT(owner_id) DO UPDATE SET agent_state_access = excluded.agent_state_access, updated_at = excluded.updated_at`,
		ownerID, allowed)
	return err
}
