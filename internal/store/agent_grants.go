// Persisted credentials issued by an owner for external agents accessing their
// library. The store receives only a SHA-256 digest of each random token.
package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
)

// AgentGrant is one owner-issued credential, minus the token itself.
type AgentGrant struct {
	ID         string     `json:"id"`
	OwnerID    int64      `json:"-"`
	Client     string     `json:"client"`
	Label      string     `json:"label"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
}

// agentGrantUseResolution is how stale last_used_at may get before a request
// rewrites it. Every request an agent makes resolves its grant, and a write
// per read would turn an agent browsing its library into a stream of writes
// to this table. The list shows "last used" to the minute, so finer is wasted.
const agentGrantUseResolution = time.Minute

// agentGrantTimeLayout is fixed-width, so the stored strings compare in time
// order and touchAgentGrant can test staleness in SQL.
const agentGrantTimeLayout = "2006-01-02T15:04:05.000Z"

const agentGrantCols = "g.id, g.owner_id, g.client, g.label, g.created_at, g.last_used_at"

func scanAgentGrant(row interface{ Scan(...any) error }) (*AgentGrant, error) {
	var g AgentGrant
	var created any
	var lastUsed sql.NullString
	if err := row.Scan(&g.ID, &g.OwnerID, &g.Client, &g.Label, &created, &lastUsed); err != nil {
		return nil, err
	}
	g.CreatedAt = anyToTime(created)
	if lastUsed.Valid {
		t := anyToTime(lastUsed.String)
		g.LastUsedAt = &t
	}
	return &g, nil
}

// CreateAgentGrant records a credential for ownerID's library. tokenHash is the
// digest used for resolution; the plaintext never reaches this package. Like
// artifacts.owner_id, this supports the no-login owner without a users row.
func (s *SQLiteStore) CreateAgentGrant(ctx context.Context, ownerID int64, label, client, tokenHash string) (*AgentGrant, error) {
	return s.createAgentGrant(ctx, ownerID, label, client, tokenHash, false)
}

// CreateAgentGrantForUser records a session owner's grant only while that user
// still exists and is enabled. The check and insert are one statement, so an
// OAuth code approved before account deletion cannot recreate an orphan grant.
func (s *SQLiteStore) CreateAgentGrantForUser(ctx context.Context, ownerID int64, label, client, tokenHash string) (*AgentGrant, error) {
	return s.createAgentGrant(ctx, ownerID, label, client, tokenHash, true)
}

func (s *SQLiteStore) createAgentGrant(ctx context.Context, ownerID int64, label, client, tokenHash string, requireUser bool) (*AgentGrant, error) {
	g := &AgentGrant{
		ID: uuid.New().String(), OwnerID: ownerID, Label: label, Client: client,
		CreatedAt: time.Now().UTC().Truncate(time.Millisecond),
	}
	query := `INSERT INTO agent_grants (id, token_hash, owner_id, label, client, created_at)
		SELECT ?1, ?2, ?3, ?4, ?5, ?6`
	if requireUser {
		query += ` WHERE EXISTS (SELECT 1 FROM users WHERE id = ?3 AND disabled_at IS NULL)`
	}
	result, err := s.db.ExecContext(ctx, query,
		g.ID, tokenHash, ownerID, label, client, g.CreatedAt.Format(agentGrantTimeLayout))
	if err != nil {
		return nil, err
	}
	if n, err := result.RowsAffected(); err != nil {
		return nil, err
	} else if n == 0 {
		return nil, ErrNotFound
	}
	return g, nil
}

// ListAgentGrants is every live grant for one owner, oldest first. It is
// the enumeration half of issuing them: an owner can revoke only what they
// can see.
func (s *SQLiteStore) ListAgentGrants(ctx context.Context, ownerID int64) ([]AgentGrant, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT "+agentGrantCols+` FROM agent_grants g
		  WHERE g.owner_id = ?
		  ORDER BY g.created_at, g.id`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AgentGrant{}
	for rows.Next() {
		g, err := scanAgentGrant(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *g)
	}
	return out, rows.Err()
}

// DeleteAgentGrant revokes one of the owner's credentials. Missing grants and
// grants belonging to another owner both return ErrNotFound.
func (s *SQLiteStore) DeleteAgentGrant(ctx context.Context, ownerID int64, grantID string) error {
	res, err := s.db.ExecContext(ctx,
		"DELETE FROM agent_grants WHERE id = ? AND owner_id = ?",
		grantID, ownerID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ResolveAgentGrant returns the grant whose token digests to tokenHash, or
// (nil, nil). Like GetSession, it answers before there is an owner to scope
// by: the grant is what says who the owner is.
//
// A grant resolves only while its owner's account, if the instance has one,
// is not disabled. Disabling an account deletes its sessions; this is the
// same refusal for its grants,
// without deleting rows the owner may want back.
//
// A successful resolve also records the use, at most once per
// agentGrantUseResolution. A failure to record it does not refuse the
// request: last_used_at is for the owner's list, not for authorization.
func (s *SQLiteStore) ResolveAgentGrant(ctx context.Context, tokenHash string) (*AgentGrant, error) {
	if tokenHash == "" {
		return nil, nil
	}
	g, err := scanAgentGrant(s.db.QueryRowContext(ctx,
		"SELECT "+agentGrantCols+` FROM agent_grants g
		   LEFT JOIN users u ON u.id = g.owner_id
		  WHERE g.token_hash = ? AND u.disabled_at IS NULL`, tokenHash))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if g.LastUsedAt == nil || now.Sub(*g.LastUsedAt) >= agentGrantUseResolution {
		if err := s.touchAgentGrant(ctx, g.ID, now); err == nil {
			g.LastUsedAt = &now
		}
	}
	return g, nil
}

func (s *SQLiteStore) touchAgentGrant(ctx context.Context, id string, now time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE agent_grants SET last_used_at = ?1
		  WHERE id = ?2 AND (last_used_at IS NULL OR last_used_at < ?3)`,
		now.Format(agentGrantTimeLayout), id,
		now.Add(-agentGrantUseResolution).Format(agentGrantTimeLayout))
	return err
}
