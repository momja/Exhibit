// Persisted agent grants (Exh-l2xj): the credential an agent outside the
// server holds to edit one artifact, issued by that artifact's owner.
//
// A grant is the durable twin of the per-session credential agentscope mints
// for a Pi sidecar, and it resolves to the same thing: one owner and one
// artifact. The store keeps only a SHA-256 of the token. Hashing is enough
// because the token is 256 random bits; a slow hash protects guessable
// secrets, and a lookup by digest leaks nothing an attacker could use.
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
	ArtifactID string     `json:"artifact_id"`
	Label      string     `json:"label"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
}

// agentGrantUseResolution is how stale last_used_at may get before a request
// rewrites it. Every request an agent makes resolves its grant, and a write
// per read would turn an agent reading its artifact into a stream of writes
// to this table. The list shows "last used" to the minute, so finer is wasted.
const agentGrantUseResolution = time.Minute

// agentGrantTimeLayout is fixed-width, so the stored strings compare in time
// order and touchAgentGrant can test staleness in SQL.
const agentGrantTimeLayout = "2006-01-02T15:04:05.000Z"

const agentGrantCols = "g.id, g.owner_id, g.artifact_id, g.label, g.created_at, g.last_used_at"

func scanAgentGrant(row interface{ Scan(...any) error }) (*AgentGrant, error) {
	var g AgentGrant
	var created any
	var lastUsed sql.NullString
	if err := row.Scan(&g.ID, &g.OwnerID, &g.ArtifactID, &g.Label, &created, &lastUsed); err != nil {
		return nil, err
	}
	g.CreatedAt = anyToTime(created)
	if lastUsed.Valid {
		t := anyToTime(lastUsed.String)
		g.LastUsedAt = &t
	}
	return &g, nil
}

// CreateAgentGrant records a credential over one of ownerID's artifacts.
// tokenHash is the digest the caller will later resolve by; the plaintext
// never reaches this package. An artifact the owner does not hold is
// ErrNotFound, exactly like one that does not exist.
func (s *SQLiteStore) CreateAgentGrant(ctx context.Context, ownerID int64, artifactID, label, tokenHash string) (*AgentGrant, error) {
	if err := s.ownsArtifact(ctx, ownerID, artifactID); err != nil {
		return nil, err
	}
	g := &AgentGrant{
		ID:         uuid.New().String(),
		OwnerID:    ownerID,
		ArtifactID: artifactID,
		Label:      label,
		CreatedAt:  time.Now().UTC().Truncate(time.Millisecond),
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO agent_grants (id, token_hash, owner_id, artifact_id, label, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		g.ID, tokenHash, ownerID, artifactID, label, g.CreatedAt.Format(agentGrantTimeLayout))
	if err != nil {
		return nil, err
	}
	return g, nil
}

// ListAgentGrants is every live grant over one artifact, oldest first. It is
// the enumeration half of issuing them: an owner can revoke only what they
// can see.
func (s *SQLiteStore) ListAgentGrants(ctx context.Context, ownerID int64, artifactID string) ([]AgentGrant, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT "+agentGrantCols+` FROM agent_grants g
		  WHERE g.artifact_id = ? AND g.owner_id = ?
		    AND EXISTS (SELECT 1 FROM artifacts a WHERE a.id = g.artifact_id AND a.owner_id = g.owner_id)
		  ORDER BY g.created_at, g.id`, artifactID, ownerID)
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

// DeleteAgentGrant revokes one grant. It is addressed by artifact as well as
// id so a grant id lifted from one artifact's list cannot revoke a grant on
// another. A missing grant, or one belonging to someone else, is ErrNotFound.
func (s *SQLiteStore) DeleteAgentGrant(ctx context.Context, ownerID int64, artifactID, grantID string) error {
	res, err := s.db.ExecContext(ctx,
		"DELETE FROM agent_grants WHERE id = ? AND artifact_id = ? AND owner_id = ?",
		grantID, artifactID, ownerID)
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
// A grant resolves only while its artifact still belongs to its owner and the
// owner's account, if the instance has one, is not disabled. Disabling an
// account deletes its sessions; this is the same refusal for its grants,
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
		   JOIN artifacts a ON a.id = g.artifact_id AND a.owner_id = g.owner_id
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
